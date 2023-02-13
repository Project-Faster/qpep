package client

import (
	"github.com/parvit/qpep/logger"
	"net"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/shared"
	"golang.org/x/net/context"
)

const (
	// INITIAL_BUFF_SIZE indicates the initial receive buffer for connections
	INITIAL_BUFF_SIZE = int64(4096)
)

var (
	// redirected indicates if the connections are using the diverter for connection
	redirected = false
	// keepRedirectionRetries counter for the number of retries to keep trying to get a connection to server
	keepRedirectionRetries = shared.DEFAULT_REDIRECT_RETRIES

	// ClientConfiguration instance of the default configuration for the client
	ClientConfiguration = ClientConfig{
		ListenHost: "0.0.0.0", ListenPort: 9443,
		GatewayHost: "198.56.1.10", GatewayPort: 443,
		RedirectedInterfaces: []int64{},
		QuicStreamTimeout:    2, MultiStream: shared.QPepConfig.MultiStream,
		MaxConnectionRetries: shared.DEFAULT_REDIRECT_RETRIES,
		IdleTimeout:          time.Duration(300) * time.Second,
		WinDivertThreads:     1,
		Verbose:              false,
	}
)

// ClientConfig struct that describes the parameter that can influence the behavior of the client
type ClientConfig struct {
	// ListenHost local address on which to listen for diverted / proxied connections
	ListenHost string
	// ListenPort local port on which to listen for diverted / proxied connections
	ListenPort int
	// GatewayHost remote address of the qpep server to which to establish quic connections
	GatewayHost string
	// GatewayPort remote port of the qpep server to which to establish quic connections
	GatewayPort int
	// RedirectedInterfaces list of ids of the interfaces that can be included for redirection
	RedirectedInterfaces []int64
	// APIPort Indicates the local/remote port of the API server (local will be 127.0.0.1:<APIPort>, remote <GatewayHost>:<APIPort>)
	APIPort int
	// QuicStreamTimeout Timeout in seconds for which to wait for a successful quic connection to the qpep server
	QuicStreamTimeout int
	// MultiStream indicates whether to enable the MultiStream option in quic-go library
	MultiStream bool
	// IdleTimeout Timeout after which, without activity, a connected quic stream is closed
	IdleTimeout time.Duration
	// MaxConnectionRetries Maximum number of tries for a qpep server connection after which the client is terminated
	MaxConnectionRetries int
	// PreferProxy If True, the first half of the MaxConnectionRetries uses the proxy instead of diverter, False is reversed
	PreferProxy bool
	// WinDivertThreads number of native threads that the windivert engine will be allowed to use
	WinDivertThreads int
	// Verbose outputs more log
	Verbose bool
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

	logger.Info("Binding to TCP %s:%d", ClientConfiguration.ListenHost, ClientConfiguration.ListenPort)
	var err error
	proxyListener, err = NewClientProxyListener("tcp", &net.TCPAddr{
		IP:   net.ParseIP(ClientConfiguration.ListenHost),
		Port: ClientConfiguration.ListenPort,
	})
	if err != nil {
		logger.Error("Encountered error when binding client proxy listener: %s", err)
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
	}()

	var connected = false
	var publicAddress = ""

	// start redirection right away because we normally expect the
	// connection with the server to be on already up
	initialCheckConnection()

	// Update loop
	for {
		select {
		case <-ctx.Done():
			if proxyListener != nil {
				proxyListener.Close()
			}
			return

		case <-time.After(1 * time.Second):
			localAddr := ClientConfiguration.ListenHost
			apiAddr := ClientConfiguration.GatewayHost
			apiPort := ClientConfiguration.APIPort
			if !connected {
				if ok, response := gatewayStatusCheck(localAddr, apiAddr, apiPort); ok {
					publicAddress = response.Address
					connected = true
					logger.Info("Server returned public address %s\n", publicAddress)

				} else {
					// if connection is lost then keep the redirection active
					// for a certain number of retries then terminate to not keep
					// all the network blocked
					if failedCheckConnection() {
						return
					}
				}
				continue
			}

			connected = clientStatisticsUpdate(localAddr, apiAddr, apiPort, publicAddress)
			if !connected {
				logger.Info("Error during statistics update from server\n")

				// if connection is lost then keep the redirection active
				// for a certain number of retries then terminate to not keep
				// all the network blocked
				if failedCheckConnection() {
					return
				}
				connected = false
			}
		}
	}
}

// initialCheckConnection method checks whether the connections checks are to initialized or not
// and honors the PreferProxy setting
func initialCheckConnection() {
	if redirected || shared.UsingProxy {
		// no need to restart, already redirected
		return
	}

	keepRedirectionRetries = ClientConfiguration.MaxConnectionRetries // reset connection tries
	preferProxy := ClientConfiguration.PreferProxy

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
	maxRetries := ClientConfiguration.MaxConnectionRetries
	preferProxy := ClientConfiguration.PreferProxy

	keepRedirectionRetries--
	if preferProxy {
		// First half of tries with proxy, then diverter, then stop
		if shared.UsingProxy && keepRedirectionRetries < maxRetries/2 {
			stopProxy()
			logger.Info("Connection failed and half retries exhausted, trying with diverter\n")
			return !initDiverter()
		}
		if keepRedirectionRetries > 0 {
			logger.Info("Connection failed, keeping redirection active (retries left: %d)\n", keepRedirectionRetries)
			stopProxy()
			initProxy()
			return false
		}

		logger.Info("Connection failed and retries exhausted, redirection stopped\n")
		stopDiverter()
		return true
	}

	// First half of tries with diverter, then proxy, then stop
	if !shared.UsingProxy && keepRedirectionRetries < maxRetries/2 {
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
	shared.AssertParamIP("listen host", shared.QPepConfig.ListenHost)
	shared.AssertParamPort("listen port", shared.QPepConfig.ListenPort)

	// copy values for client configuration
	ClientConfiguration.GatewayHost = shared.QPepConfig.GatewayHost
	ClientConfiguration.GatewayPort = shared.QPepConfig.GatewayPort
	ClientConfiguration.APIPort = shared.QPepConfig.GatewayAPIPort
	ClientConfiguration.ListenHost, ClientConfiguration.RedirectedInterfaces = shared.GetDefaultLanListeningAddress(
		shared.QPepConfig.ListenHost, shared.QPepConfig.GatewayHost)
	ClientConfiguration.ListenPort = shared.QPepConfig.ListenPort
	ClientConfiguration.MaxConnectionRetries = shared.QPepConfig.MaxConnectionRetries
	ClientConfiguration.MultiStream = shared.QPepConfig.MultiStream
	ClientConfiguration.WinDivertThreads = shared.QPepConfig.WinDivertThreads
	ClientConfiguration.PreferProxy = shared.QPepConfig.PreferProxy
	ClientConfiguration.Verbose = shared.QPepConfig.Verbose

	// panic if configuration is inconsistent
	shared.AssertParamIP("gateway host", ClientConfiguration.GatewayHost)
	shared.AssertParamPort("gateway port", ClientConfiguration.GatewayPort)

	shared.AssertParamPort("api port", ClientConfiguration.APIPort)

	shared.AssertParamNumeric("max connection retries", ClientConfiguration.MaxConnectionRetries, 1, 300)
	shared.AssertParamNumeric("max diverter threads", ClientConfiguration.WinDivertThreads, 1, 32)

	shared.AssertParamHostsDifferent("hosts", ClientConfiguration.GatewayHost, ClientConfiguration.ListenHost)
	shared.AssertParamPortsDifferent("ports", ClientConfiguration.GatewayPort,
		ClientConfiguration.ListenPort, ClientConfiguration.APIPort)

	shared.AssertParamNumeric("auto-redirected interfaces", len(ClientConfiguration.RedirectedInterfaces), 0, 256)

	// validation ok
	logger.Info("Client configuration validation OK\n")
}
