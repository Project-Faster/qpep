package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"github.com/Project-Faster/quic-go"
	//"github.com/Project-Faster/quic-go/logging"
	//"github.com/Project-Faster/quic-go/qlog"
	"io/ioutil"
	"math/big"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
)

var (
	// ServerConfiguration global variable that keeps track of the current server configuration
	ServerConfiguration = ServerConfig{
		ListenHost:  "0.0.0.0",
		ListenPort:  443,
		APIPort:     444,
		IdleTimeout: 30 * time.Second,
	}
	// quicListener instance of the quic server that receives the connections from clients
	quicListener quic.Listener
)

// ServerConfiguration struct models the parameters necessary for running the quic server
type ServerConfig struct {
	// ListenHost ip address on which the server listens for connections
	ListenHost string
	// ListenPort port [1-65535] on which the server listens for connections
	ListenPort int
	// APIPort port [1-65535] on which the API server is launched
	APIPort int
	// IdleTimeout Timeout after which, without activity, a connected quic stream is closed
	IdleTimeout time.Duration

	BrokerConfig shared.AnalyticsDefinition
}

// RunServer method validates the provided server configuration and then launches the server
// with the input context
func RunServer(ctx context.Context, cancel context.CancelFunc) {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v\n", err)
			debug.PrintStack()
		}
		if quicListener != nil {
			quicListener.Close()
		}
		cancel()
	}()

	// update configuration from flags
	validateConfiguration()

	if ServerConfiguration.BrokerConfig.Enabled {
		api.Statistics.Start(&ServerConfiguration.BrokerConfig)
	}
	defer api.Statistics.Stop()

	listenAddr := ServerConfiguration.ListenHost + ":" + strconv.Itoa(ServerConfiguration.ListenPort)
	logger.Info("Opening QPEP Server on: %s\n", listenAddr)
	var err error
	quicListener, err = quic.ListenAddr(listenAddr, generateTLSConfig(), shared.GetQuicConfiguration())
	if err != nil {
		logger.Info("Encountered error while binding QUIC listener: %s\n", err)
		return
	}

	// launches listener
	go listenQuicSession()

	ctxPerfWatcher, perfWatcherCancel := context.WithCancel(context.Background())
	go performanceWatcher(ctxPerfWatcher)

	// termination loop
	for {
		select {
		case <-ctx.Done():
			perfWatcherCancel()
			return
		case <-time.After(10 * time.Millisecond):
			continue
		}
	}
}

// generateTLSConfig creates a new x509 key/certificate pair and dumps it to the disk
func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	ioutil.WriteFile("server_key.pem", keyPEM, 0777)
	ioutil.WriteFile("server_cert.pem", certPEM, 0777)

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"qpep"},
	}
}

// performanceWatcher method is a goroutine that checks the current speed of every host every second and
// updates the values for the current speed and total number of bytes uploaded / downloaded
func performanceWatcher(ctx context.Context) {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v\n", err)
			debug.PrintStack()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
			hosts := api.Statistics.GetHosts()

			for _, host := range hosts {
				// load the current count and reset it atomically (so there's no race condition)
				dwCount := api.Statistics.GetCounterAndClear(api.PERF_DW_COUNT, host)
				upCount := api.Statistics.GetCounterAndClear(api.PERF_UP_COUNT, host)

				// update the speeds and totals for the client
				if dwCount >= 0.0 {
					api.Statistics.SetCounter(dwCount/1024.0, api.PERF_DW_SPEED, host)
					api.Statistics.IncrementCounter(dwCount, api.PERF_DW_TOTAL, host)
				}

				if upCount >= 0.0 {
					api.Statistics.SetCounter(upCount/1024.0, api.PERF_UP_SPEED, host)
					api.Statistics.IncrementCounter(upCount, api.PERF_UP_TOTAL, host)
				}
			}
		}
	}
}

// validateConfiguration method checks the validity of the configuration values that have been provided, panicking if
// there is any issue
func validateConfiguration() {
	shared.AssertParamIP("listen host", shared.QPepConfig.ListenHost)

	ServerConfiguration.ListenHost, _ = shared.GetDefaultLanListeningAddress(shared.QPepConfig.ListenHost, "")
	ServerConfiguration.ListenPort = shared.QPepConfig.ListenPort
	ServerConfiguration.APIPort = shared.QPepConfig.GatewayAPIPort

	shared.AssertParamPort("listen port", ServerConfiguration.ListenPort)

	shared.AssertParamPort("api port", ServerConfiguration.APIPort)

	shared.AssertParamPortsDifferent("ports", ServerConfiguration.ListenPort, ServerConfiguration.APIPort)

	brokerConfig := shared.QPepConfig.Analytics
	if !brokerConfig.Enabled {
		ServerConfiguration.BrokerConfig.Enabled = false
	} else {
		shared.AssertParamIP("broker address", brokerConfig.BrokerAddress)
		shared.AssertParamPort("broker port", brokerConfig.BrokerPort)
		shared.AssertParamString("broker topic", brokerConfig.BrokerTopic)
		shared.AssertParamChoice("broker protocol", brokerConfig.BrokerProtocol, []string{"tcp", "udp"})

		ServerConfiguration.BrokerConfig.Enabled = true
		ServerConfiguration.BrokerConfig.BrokerAddress = brokerConfig.BrokerAddress
		ServerConfiguration.BrokerConfig.BrokerPort = brokerConfig.BrokerPort
		ServerConfiguration.BrokerConfig.BrokerProtocol = brokerConfig.BrokerProtocol
		ServerConfiguration.BrokerConfig.BrokerTopic = brokerConfig.BrokerTopic
	}

	logger.Info("Server configuration validation OK\n")
}
