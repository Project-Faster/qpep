package client

import (
	"github.com/Project-Faster/qpep/shared/configuration"
	"github.com/Project-Faster/qpep/shared/logger"
	"github.com/Project-Faster/qpep/workers/gateway"
	"net"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/Project-Faster/qpep/api"
	"github.com/Project-Faster/qpep/shared"
	"golang.org/x/net/context"
)

const (
	// INITIAL_BUFF_SIZE indicates the initial receive buffer for connections
	INITIAL_BUFF_SIZE = int64(4096)
)

var (
	GATEWAY_CHECK_WAIT = 10 * time.Second

	// redirected indicates if the connections are using the diverter for connection
	redirected = false
	// keepRedirectionRetries counter for the number of retries to keep trying to get a connection to server
	keepRedirectionRetries = configuration.DEFAULT_REDIRECT_RETRIES

	// clientAdditional instance of the default configuration for the client
	clientAdditional = ClientConfig{
		RedirectedInterfaces: []int64{},
		QuicStreamTimeout:    2,
		IdleTimeout:          time.Duration(3) * time.Second,
	}
)

// ClientConfig struct that describes the parameter that can influence the behavior of the client
type ClientConfig struct {
	// RedirectedInterfaces list of ids of the interfaces that can be included for redirection
	RedirectedInterfaces []int64
	// QuicStreamTimeout Timeout in seconds for which to wait for a successful quic connection to the qpep server
	QuicStreamTimeout int
	// IdleTimeout Timeout after which, without activity, a connected quic stream is closed
	IdleTimeout time.Duration
}

// RunClient method executes the qpep in client mode and initializes its services
func RunClient(ctx context.Context, cancel context.CancelFunc) {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v", err)
			debug.PrintStack()
		}
		if proxyListener != nil {
			proxyListener.Close()
		}
		cancel()
	}()
	logger.Info("Starting TCP-QPEP Tunnel Listener")

	// update configuration from flags
	validateConfiguration()

	config := configuration.QPepConfig.Client

	logger.Info("Binding to TCP %s:%d", config.LocalListeningAddress, config.LocalListenPort)
	var err error
	proxyListener, err = NewClientProxyListener("tcp", &net.TCPAddr{
		IP:   net.ParseIP(config.LocalListeningAddress),
		Port: config.LocalListenPort,
	})
	if err != nil {
		logger.Error("Encountered error when binding client proxy listener: %s", err)
		var errPtr = ctx.Value("lastError").(*error)
		*errPtr = err
		return
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go listenTCPConn(wg)
	go handleServices(ctx, cancel, wg)

	wg.Wait()
}

// handleServices method encapsulates the logic for checking the connection to the server
// by executing API calls
func handleServices(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup) {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v", err)
			debug.PrintStack()
		}
		wg.Done()
		cancel()

		stopDiverter()
		stopProxy()
	}()

	var connected = false
	var checkIsRunning = false

	// start redirection right away because we normally expect the
	// connection with the server to be on already up
	initialCheckConnection()

	config := configuration.QPepConfig

	localAddr := config.Client.LocalListeningAddress
	apiAddr := config.Client.GatewayHost
	apiPort := config.General.APIPort
	connected, _ = gatewayStatusCheck(localAddr, apiAddr, apiPort)

	// Update loop
	for {
		select {
		case <-ctx.Done():
			if proxyListener != nil {
				proxyListener.Close()
			}
			return

		case <-time.After(GATEWAY_CHECK_WAIT):
			if shared.DEBUG_MASK_REDIRECT || checkIsRunning {
				continue
			}
			checkIsRunning = true
			connected, _ = gatewayStatusCheck(localAddr, apiAddr, apiPort)
			checkIsRunning = false

			if !connected {
				// if connection is lost then keep the redirection active
				// for a certain number of retries then terminate to not keep
				// all the network blocked
				if failedCheckConnection() {
					return
				}
			}
		}
	}
}

// initialCheckConnection method checks whether the connections checks are to initialized or not
// and honors the PreferProxy setting
func initialCheckConnection() {
	if redirected || gateway.UsingProxy {
		// no need to restart, already redirected
		return
	}

	configGeneral := configuration.QPepConfig.General

	keepRedirectionRetries = configGeneral.MaxConnectionRetries // reset connection tries
	preferProxy := configGeneral.PreferProxy

	if preferProxy {
		logger.Info("Proxy preference set, trying to connect...\n")
		initProxy()
		return
	}

	initDiverter()
}

// failedCheckConnection method handles the logic for switching between diverter and proxy (or viceversa if PreferProxy true)
// after half connection tries are failed, and stopping altogether if retries are exhausted
func failedCheckConnection() bool {
	configGeneral := configuration.QPepConfig.General

	maxRetries := configGeneral.MaxConnectionRetries
	preferProxy := configGeneral.PreferProxy

	gateway.ScaleUpTimeout()

	keepRedirectionRetries--
	if preferProxy {
		// First half of tries with proxy, then diverter, then stop
		if gateway.UsingProxy && keepRedirectionRetries < maxRetries/2 {
			stopProxy()
			logger.Info("Connection failed and half retries exhausted, trying with diverter\n")
			return !initDiverter()
		}
		if keepRedirectionRetries > 0 {
			logger.Info("Connection failed, keeping redirection active (retries left: %d)\n", keepRedirectionRetries)
			//stopProxy()
			initProxy()
			return false
		}

		logger.Info("Connection failed and retries exhausted, redirection stopped\n")
		stopDiverter()
		return true
	}

	// First half of tries with diverter, then proxy, then stop
	if !gateway.UsingProxy && keepRedirectionRetries < maxRetries/2 {
		stopDiverter()
		logger.Info("Connection failed and half retries exhausted, trying with proxy\n")
		initProxy()
		return false
	}
	if keepRedirectionRetries > 0 {
		logger.Info("Connection failed, keeping redirection active (retries left: %d)\n", keepRedirectionRetries)
		stopDiverter()
		initDiverter()
		return false
	}

	logger.Info("Connection failed and retries exhausted, redirection stopped\n")
	stopProxy()
	return true
}

// gatewayStatusCheck wraps the request for the /echo API to the api server
func gatewayStatusCheck(localAddr, apiAddr string, apiPort int) (bool, *api.EchoResponse) {
	if response := api.RequestEcho(localAddr, apiAddr, apiPort, true); response != nil {
		logger.Info("Gateway Echo OK\n")
		return true, response
	}
	logger.Info("Gateway Echo FAILED\n")
	return false, nil
}

// clientStatisticsUpdate wraps the request for the /statistics/data API to the api server, and updates the local statistics
// with the ones received
func clientStatisticsUpdate(localAddr, apiAddr string, apiPort int, publicAddress string) bool {
	response := api.RequestStatistics(localAddr, apiAddr, apiPort, publicAddress)
	if response == nil {
		logger.Info("Statistics update failed, resetting connection status\n")
		return false
	}

	for _, stat := range response.Data {
		value, err := strconv.ParseFloat(stat.Value, 64)
		if err != nil {
			continue
		}
		api.Statistics.SetCounter(value, stat.Name)
	}
	return true
}

// validateConfiguration method handles the checking of the configuration values provided in the configuration files
// for the client mode
func validateConfiguration() {
	configClient := configuration.QPepConfig.Client
	configGeneral := configuration.QPepConfig.General
	configProto := configuration.QPepConfig.Protocol

	configuration.AssertParamIP("listen host", configClient.LocalListeningAddress)
	configuration.AssertParamPort("listen port", configClient.LocalListenPort)

	configuration.AssertParamNumeric("buffer size", configProto.BufferSize, 1, 1024)

	// resolve local listening address
	configClient.LocalListeningAddress, clientAdditional.RedirectedInterfaces =
		gateway.GetDefaultLanListeningAddress(configClient.LocalListeningAddress, configClient.GatewayHost)

	// panic if configuration is inconsistent
	configuration.AssertParamIP("gateway host", configClient.GatewayHost)
	configuration.AssertParamPort("gateway port", configClient.GatewayPort)

	configuration.AssertParamPort("api port", configGeneral.APIPort)

	configuration.AssertParamNumeric("max connection retries", configGeneral.MaxConnectionRetries, 1, 300)
	configuration.AssertParamNumeric("max diverter threads", configGeneral.WinDivertThreads, 1, 32)
	configuration.AssertParamValidTimeout("idle timeout", configProto.IdleTimeout)

	configuration.AssertParamHostsDifferent("hosts", configClient.GatewayHost, configClient.LocalListeningAddress)
	configuration.AssertParamPortsDifferent("ports", configClient.GatewayPort,
		configClient.LocalListenPort, configGeneral.APIPort)

	configuration.AssertParamNumeric("auto-redirected interfaces", len(clientAdditional.RedirectedInterfaces), 0, 256)

	switch runtime.GOOS {
	case "linux":
		logger.Info("Platform forced prefer_proxy to false\n")
		configGeneral.PreferProxy = false
		break
	}

	// validation ok
	logger.Info("Client configuration validation OK\n")
}
