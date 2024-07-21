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
	"github.com/Project-Faster/qpep/shared"
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
	suite.Run(t, &q)
}

type ClientSuite struct {
	suite.Suite

	mtx sync.Mutex
}

func (s *ClientSuite) BeforeTest(_, testName string) {
	shared.SetSystemProxy(false)
	api.Statistics.Reset()
	proxyListener = nil

	shared.UsingProxy = false
	shared.ProxyAddress = nil

	shared.QPepConfig.GatewayHost = "127.0.0.1"
	shared.QPepConfig.GatewayPort = 9443
	shared.QPepConfig.GatewayAPIPort = 445
	shared.QPepConfig.ListenHost = "127.0.0.1"
	shared.QPepConfig.ListenPort = 9090

	shared.QPepConfig.MaxConnectionRetries = 15
	shared.QPepConfig.MultiStream = true
	shared.QPepConfig.WinDivertThreads = 4
	shared.QPepConfig.PreferProxy = true
	shared.QPepConfig.Verbose = true
}

func (s *ClientSuite) AfterTest(_, testName string) {
	monkey.UnpatchAll()
	shared.SetSystemProxy(false)
}

func (s *ClientSuite) TestValidateConfiguration() {
	assert.NotPanics(s.T(), func() {
		validateConfiguration()
	})
}

func (s *ClientSuite) TestValidateConfiguration_BadGatewayHost() {
	shared.QPepConfig.GatewayHost = ""
	assert.PanicsWithValue(s.T(), shared.ErrConfigurationValidationFailed,
		func() {
			validateConfiguration()
		})
}

func (s *ClientSuite) TestValidateConfiguration_BadGatewayPort() {
	for _, val := range []int{0, -1, 99999} {
		shared.QPepConfig.GatewayPort = val
		assert.PanicsWithValue(s.T(), shared.ErrConfigurationValidationFailed,
			func() {
				validateConfiguration()
			})
	}
}

func (s *ClientSuite) TestValidateConfiguration_BadListenHost() {
	shared.QPepConfig.ListenHost = ""
	assert.PanicsWithValue(s.T(), shared.ErrConfigurationValidationFailed,
		func() {
			validateConfiguration()
		})
}

func (s *ClientSuite) TestValidateConfiguration_BadListenPort() {
	for _, val := range []int{0, -1, 99999} {
		shared.QPepConfig.ListenPort = val
		assert.PanicsWithValue(s.T(), shared.ErrConfigurationValidationFailed,
			func() {
				validateConfiguration()
			})
	}
}

func (s *ClientSuite) TestValidateConfiguration_BadConnectionRetries() {
	for _, val := range []int{0, -1, 99999} {
		shared.QPepConfig.MaxConnectionRetries = val
		assert.PanicsWithValue(s.T(), shared.ErrConfigurationValidationFailed,
			func() {
				validateConfiguration()
			})
	}
}

func (s *ClientSuite) TestValidateConfiguration_BadDiverterThreads() {
	for _, val := range []int{0, -1, 64} {
		shared.QPepConfig.WinDivertThreads = val
		assert.PanicsWithValue(s.T(), shared.ErrConfigurationValidationFailed,
			func() {
				validateConfiguration()
			})
	}
}

func (s *ClientSuite) TestRunClient_ErrorListener() {
	validateConfiguration()
	proxyListener, _ = NewClientProxyListener("tcp", &net.TCPAddr{
		IP:   net.ParseIP(ClientConfiguration.ListenHost),
		Port: ClientConfiguration.ListenPort,
	})

	ClientConfiguration.ListenPort = 0

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

		HandshakeIdleTimeout: shared.GetScaledTimeout(10, time.Second),
		KeepAlivePeriod:      0,

		EnableDatagrams: false,
	}

	listener, _ := quic.ListenAddr(
		fmt.Sprintf("%s:%d", shared.QPepConfig.GatewayHost, shared.QPepConfig.GatewayPort),
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
		qpepHeader, err := shared.QPepHeaderFromBytes(stream)
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
