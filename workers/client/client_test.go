package client

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/Project-Faster/monkey"
	"github.com/Project-Faster/qpep/api"
	"github.com/Project-Faster/qpep/shared/configuration"
	"github.com/Project-Faster/qpep/shared/errors"
	"github.com/Project-Faster/qpep/shared/protocol"
	"github.com/Project-Faster/qpep/workers/gateway"
	"github.com/Project-Faster/quic-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"
)

func TestClientSuite(t *testing.T) {
	var q ClientSuite
	q.testLocalListenPort = 9090
	suite.Run(t, &q)
}

type ClientSuite struct {
	suite.Suite

	testLocalListenPort int

	mtx sync.Mutex
}

func (s *ClientSuite) BeforeTest(_, testName string) {
	proxyListener = nil

	s.testLocalListenPort++

	gateway.UsingProxy = false
	gateway.ProxyAddress = nil

	GATEWAY_CHECK_WAIT = 1 * time.Second

	configuration.QPepConfig = configuration.QPepConfigType{}
	configuration.QPepConfig.Merge(&configuration.DefaultConfig)

	configuration.QPepConfig.Client.GatewayHost = "127.0.0.2"
	configuration.QPepConfig.Client.GatewayPort = 9443
	configuration.QPepConfig.Client.LocalListeningAddress = "127.0.0.1"
	configuration.QPepConfig.Client.LocalListenPort = s.testLocalListenPort

	configuration.QPepConfig.General.APIPort = 445
	configuration.QPepConfig.General.MaxConnectionRetries = 15
	configuration.QPepConfig.General.MultiStream = true
	configuration.QPepConfig.General.WinDivertThreads = 4
	configuration.QPepConfig.General.PreferProxy = true
	configuration.QPepConfig.General.Verbose = false

	gateway.SetSystemProxy(false)
	api.Statistics.Reset()
}

func (s *ClientSuite) AfterTest(_, testName string) {
	monkey.UnpatchAll()
	gateway.SetSystemProxy(false)
}

func (s *ClientSuite) TestValidateConfiguration() {
	assert.NotPanics(s.T(), func() {
		validateConfiguration()
	})
}

func (s *ClientSuite) TestValidateConfiguration_BadGatewayHost() {
	configuration.QPepConfig.Client.GatewayHost = ""
	assert.PanicsWithValue(s.T(), errors.ErrConfigurationValidationFailed,
		func() {
			validateConfiguration()
		})
}

func (s *ClientSuite) TestValidateConfiguration_BadGatewayPort() {
	for _, val := range []int{0, -1, 99999} {
		configuration.QPepConfig.Client.GatewayPort = val
		assert.PanicsWithValue(s.T(), errors.ErrConfigurationValidationFailed,
			func() {
				validateConfiguration()
			})
	}
}

func (s *ClientSuite) TestValidateConfiguration_BadListenHost() {
	configuration.QPepConfig.Client.LocalListeningAddress = ""
	assert.PanicsWithValue(s.T(), errors.ErrConfigurationValidationFailed,
		func() {
			validateConfiguration()
		})
}

func (s *ClientSuite) TestValidateConfiguration_BadListenPort() {
	for _, val := range []int{0, -1, 99999} {
		configuration.QPepConfig.Client.LocalListenPort = val
		assert.PanicsWithValue(s.T(), errors.ErrConfigurationValidationFailed,
			func() {
				validateConfiguration()
			})
	}
}

func (s *ClientSuite) TestValidateConfiguration_BadConnectionRetries() {
	for _, val := range []int{0, -1, 99999} {
		configuration.QPepConfig.General.MaxConnectionRetries = val
		assert.PanicsWithValue(s.T(), errors.ErrConfigurationValidationFailed,
			func() {
				validateConfiguration()
			})
	}
}

func (s *ClientSuite) TestValidateConfiguration_BadDiverterThreads() {
	for _, val := range []int{0, -1, 64} {
		configuration.QPepConfig.General.WinDivertThreads = val
		assert.PanicsWithValue(s.T(), errors.ErrConfigurationValidationFailed,
			func() {
				validateConfiguration()
			})
	}
}

func (s *ClientSuite) TestRunClient_ErrorListener() {
	validateConfiguration()
	proxyListener, _ = NewClientProxyListener("tcp", &net.TCPAddr{
		IP:   net.ParseIP(configuration.QPepConfig.Client.LocalListeningAddress),
		Port: configuration.QPepConfig.Client.LocalListenPort,
	})

	configuration.QPepConfig.Client.LocalListenPort = 0

	ctx, cancel := context.WithCancel(context.Background())
	assert.NotPanics(s.T(), func() {
		RunClient(ctx, cancel)
	})

	assert.NotNil(s.T(), <-ctx.Done())
}

// --- utilities --- //
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

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"qpep"},
	}
}

func fakeQuicListener(ctx context.Context, cancel context.CancelFunc, t *testing.T, wg *sync.WaitGroup) {
	defer func() {
		if err := recover(); err != nil {
			t.Fatalf("PANIC: %v\n", err)
		}
	}()
	tlsConfig := generateTLSConfig()
	quicClientConfig := &quic.Config{
		MaxIncomingStreams:      1024,
		DisablePathMTUDiscovery: true,
		MaxIdleTimeout:          3 * time.Second,

		HandshakeIdleTimeout: gateway.GetScaledTimeout(10, time.Second),
		KeepAlivePeriod:      0,

		EnableDatagrams: false,
	}

	listener, _ := quic.ListenAddr(
		fmt.Sprintf("%s:%d", configuration.QPepConfig.Client.GatewayHost, configuration.QPepConfig.Client.GatewayPort),
		tlsConfig, quicClientConfig)
	defer func() {
		_ = listener.Close()
		wg.Done()
		cancel()
	}()
	for {
		session, err := listener.Accept(ctx)
		if err != nil {
			return
		}

		stream, _ := session.AcceptStream(context.Background())
		qpepHeader, err := protocol.QPepHeaderFromBytes(stream)
		if err != nil {
			_ = stream.Close()
			return
		}

		data, _ := json.Marshal(qpepHeader)

		stream.SetWriteDeadline(time.Now().Add(1 * time.Second))
		n, _ := stream.Write(data)
		t.Logf("n: %d\n", n)

		_ = stream.Close()
	}
}
