package client

import (
	"bou.ke/monkey"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/Project-Faster/quic-go"
	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/shared/configuration"
	"github.com/parvit/qpep/shared/errors"
	"github.com/parvit/qpep/shared/protocol"
	"github.com/parvit/qpep/workers/gateway"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"math/big"
	"net"
	"net/url"
	"runtime"
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
	gateway.SetSystemProxy(false)
	api.Statistics.Reset()
	proxyListener = nil

	gateway.UsingProxy = false
	gateway.ProxyAddress = nil

	GATEWAY_CHECK_WAIT = 1 * time.Second

	configuration.QPepConfig = configuration.QPepConfigType{}
	configuration.QPepConfig.Merge(&configuration.DefaultConfig)

	configuration.QPepConfig.Client.GatewayHost = "127.0.0.1"
	configuration.QPepConfig.Client.GatewayPort = 9443
	configuration.QPepConfig.Client.LocalListeningAddress = "127.0.0.1"
	configuration.QPepConfig.Client.LocalListenPort = 9090

	configuration.QPepConfig.General.APIPort = 445
	configuration.QPepConfig.General.MaxConnectionRetries = 15
	configuration.QPepConfig.General.MultiStream = true
	configuration.QPepConfig.General.WinDivertThreads = 4
	configuration.QPepConfig.General.PreferProxy = true
	configuration.QPepConfig.General.Verbose = false
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

func (s *ClientSuite) TestRunClient() {
	var calledNewProxy = false
	monkey.Patch(NewClientProxyListener, func(string, *net.TCPAddr) (net.Listener, error) {
		calledNewProxy = true
		return nil, nil
	})
	var calledListen = false
	monkey.Patch(listenTCPConn, func(wg *sync.WaitGroup) {
		defer wg.Done()
		<-time.After(1 * time.Second)
		calledListen = true
	})
	var calledHandle = false
	monkey.Patch(handleServices, func(_ context.Context, _ context.CancelFunc, wg *sync.WaitGroup) {
		defer wg.Done()
		<-time.After(1 * time.Second)
		calledHandle = true
	})

	ctx, cancel := context.WithCancel(context.Background())
	RunClient(ctx, cancel)

	assert.True(s.T(), calledNewProxy)
	assert.True(s.T(), calledListen)
	assert.True(s.T(), calledHandle)

	assert.NotNil(s.T(), <-ctx.Done())
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

func (s *ClientSuite) TestHandleServices() {
	proxyListener, _ = net.Listen("tcp", "127.0.0.1:9090")
	defer func() {
		if proxyListener != nil {
			_ = proxyListener.Close()
		}
	}()

	wg2 := &sync.WaitGroup{}
	wg2.Add(2)

	var calledInitialCheck = false
	monkey.Patch(initialCheckConnection, func() {
		if !calledInitialCheck {
			calledInitialCheck = true
			wg2.Done()
		}
	})
	var calledGatewayCheck = false
	monkey.Patch(gatewayStatusCheck, func(string, string, int) (bool, *api.EchoResponse) {
		if !calledGatewayCheck {
			calledGatewayCheck = true
			wg2.Done()
		}
		return true, &api.EchoResponse{
			Address:       "172.20.50.150",
			Port:          54635,
			ServerVersion: "0.1.0",
		}
	})

	wg := &sync.WaitGroup{}
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	go handleServices(ctx, cancel, wg)

	ch := make(chan struct{})

	go func() {
		wg2.Wait()
		cancel()
		wg.Wait()

		ch <- struct{}{}
	}()

	select {
	case <-time.After((GATEWAY_CHECK_WAIT * 3) + (1 * time.Second)):
		s.T().Logf("Test Timed out waiting for routines to finish")
		s.T().FailNow()
		return
	case <-ch:
		break
	}

	assert.True(s.T(), calledInitialCheck)
	assert.True(s.T(), calledGatewayCheck)
}

func (s *ClientSuite) TestHandleServices_PanicCheck() {
	var calledInitialCheck = false
	monkey.Patch(initialCheckConnection, func() {
		calledInitialCheck = true
		panic("test-error")
	})

	wg := &sync.WaitGroup{}
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	go handleServices(ctx, cancel, wg)

	cancel()
	wg.Wait()

	assert.True(s.T(), calledInitialCheck)
}

func (s *ClientSuite) TestHandleServices_FailGateway() {
	wg2 := &sync.WaitGroup{}
	wg2.Add(3)
	var calledInitialCheck = false
	monkey.Patch(initialCheckConnection, func() {
		if !calledInitialCheck {
			calledInitialCheck = true
			wg2.Done()
		}
	})
	var calledGatewayCheck = false
	monkey.Patch(gatewayStatusCheck, func(string, string, int) (bool, *api.EchoResponse) {
		if !calledGatewayCheck {
			calledGatewayCheck = true
			wg2.Done()
		}
		return false, nil
	})
	var calledFailedConnectionFirst = false
	var calledFailedConnectionSecond = false
	monkey.Patch(failedCheckConnection, func() bool {
		if !calledFailedConnectionFirst {
			calledFailedConnectionFirst = true
			return false
		}
		if !calledFailedConnectionSecond {
			calledFailedConnectionSecond = true
			wg2.Done()
		}
		return true
	})

	wg := &sync.WaitGroup{}
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	go handleServices(ctx, cancel, wg)

	ch := make(chan struct{})

	go func() {
		wg2.Wait()
		cancel()
		wg.Wait()

		ch <- struct{}{}
	}()

	select {
	case <-time.After((GATEWAY_CHECK_WAIT * 3) + (1 * time.Second)):
		s.T().Logf("Test Timed out waiting for routines to finish")
		s.T().FailNow()
		return
	case <-ch:
		break
	}

	assert.True(s.T(), calledInitialCheck)
	assert.True(s.T(), calledGatewayCheck)
	assert.True(s.T(), calledFailedConnectionFirst)
	assert.True(s.T(), calledFailedConnectionSecond)
}

func (s *ClientSuite) TestInitProxy() {
	monkey.Patch(gateway.SetSystemProxy, func(active bool) {
		assert.True(s.T(), active)
		gateway.UsingProxy = true
		gateway.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
	})

	assert.False(s.T(), gateway.UsingProxy)
	initProxy()
	assert.True(s.T(), gateway.UsingProxy)

	assert.NotNil(s.T(), gateway.ProxyAddress)
}

func (s *ClientSuite) TestStopProxy() {
	gateway.UsingProxy = true
	gateway.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")

	monkey.Patch(gateway.SetSystemProxy, func(active bool) {
		assert.False(s.T(), active)
		gateway.UsingProxy = false
		gateway.ProxyAddress = nil
	})

	assert.True(s.T(), gateway.UsingProxy)
	stopProxy()
	assert.False(s.T(), gateway.UsingProxy)
	assert.False(s.T(), redirected)

	assert.Nil(s.T(), gateway.ProxyAddress)
}

func (s *ClientSuite) TestInitDiverter() {
	redirected = false
	monkey.Patch(gateway.SetConnectionDiverter, func(active bool, _ string, _ string, _ int, _ int, _ int, _ int64, _ []int) bool {
		assert.True(s.T(), active)
		redirected = true
		return true
	})
	assert.True(s.T(), initDiverter())
	assert.True(s.T(), redirected)
}

func (s *ClientSuite) TestInitDiverter_Fail() {
	redirected = true
	monkey.Patch(gateway.SetConnectionDiverter, func(active bool, _ string, _ string, _ int, _ int, _ int, _ int64, _ []int) bool {
		assert.True(s.T(), active)
		redirected = false
		return false
	})
	assert.False(s.T(), initDiverter())
	assert.False(s.T(), redirected)
}

func (s *ClientSuite) TestStopDiverter() {
	redirected = true
	monkey.Patch(gateway.SetConnectionDiverter, func(active bool, _ string, _ string, _ int, _ int, _ int, _ int64, _ []int) bool {
		assert.False(s.T(), active)
		redirected = false
		return false
	})
	stopDiverter()
	assert.False(s.T(), redirected)
}

func (s *ClientSuite) TestInitialCheckConnection_Proxy() {
	if runtime.GOOS == "linux" {
		s.T().Skipf("Proxy set not supported on linux")
		return
	}

	configuration.QPepConfig.General.PreferProxy = true
	validateConfiguration()

	monkey.Patch(gateway.SetSystemProxy, func(active bool) {
		assert.True(s.T(), active)
		gateway.UsingProxy = true
		gateway.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
	})

	redirected = false
	assert.False(s.T(), gateway.UsingProxy)
	initialCheckConnection()
	assert.True(s.T(), gateway.UsingProxy)
	assert.NotNil(s.T(), gateway.ProxyAddress)

	assert.False(s.T(), redirected)
}

func (s *ClientSuite) TestInitialCheckConnection_Diverter() {
	configuration.QPepConfig.General.PreferProxy = false
	validateConfiguration()
	if runtime.GOOS == "linux" {
		assert.False(s.T(), configuration.QPepConfig.General.PreferProxy)
	}

	redirected = false
	monkey.Patch(initDiverter, func() bool {
		redirected = true
		return true
	})

	assert.False(s.T(), gateway.UsingProxy)
	initialCheckConnection()
	assert.False(s.T(), gateway.UsingProxy)
	assert.Nil(s.T(), gateway.ProxyAddress)

	assert.True(s.T(), redirected)
}

func (s *ClientSuite) TestGatewayStatusCheck() {
	monkey.Patch(api.RequestEcho, func(string, string, int, bool) *api.EchoResponse {
		return &api.EchoResponse{
			Address:       "127.0.0.1",
			Port:          9090,
			ServerVersion: "0.1.0",
		}
	})

	ok, resp := gatewayStatusCheck("127.0.0.1", "127.0.0.1", 8080)
	assert.True(s.T(), ok)
	assert.Equal(s.T(), "127.0.0.1", resp.Address)
	assert.Equal(s.T(), int64(9090), resp.Port)
	assert.Equal(s.T(), "0.1.0", resp.ServerVersion)
}

func (s *ClientSuite) TestGatewayStatusCheck_Fail() {
	monkey.Patch(api.RequestEcho, func(string, string, int, bool) *api.EchoResponse {
		return nil
	})

	ok, resp := gatewayStatusCheck("127.0.0.1", "127.0.0.1", 8080)
	assert.False(s.T(), ok)
	assert.Nil(s.T(), resp)
}

func (s *ClientSuite) TestClientStatisticsUpdate() {
	monkey.Patch(api.RequestStatistics, func(string, string, int, string) *api.StatsInfoResponse {
		return &api.StatsInfoResponse{
			Data: []api.StatsInfo{
				{
					ID:        1,
					Attribute: "Current Connections",
					Value:     "30",
					Name:      api.PERF_CONN,
				},
				{
					ID:        2,
					Attribute: "Current Upload Speed",
					Value:     "0.99",
					Name:      api.PERF_UP_SPEED,
				},
				{
					ID:        3,
					Attribute: "Current Download Speed",
					Value:     "1.00",
					Name:      api.PERF_DW_SPEED,
				},
				{
					ID:        4,
					Attribute: "Total Uploaded Bytes",
					Value:     "100.00",
					Name:      api.PERF_UP_TOTAL,
				},
				{
					ID:        5,
					Attribute: "Total Downloaded Bytes",
					Value:     "10.00",
					Name:      api.PERF_DW_TOTAL,
				},
			},
		}
	})

	ok := clientStatisticsUpdate("127.0.0.1", "127.0.0.1", 8080, "172.30.54.250")
	assert.True(s.T(), ok)

	assert.Equal(s.T(), 30.0, api.Statistics.GetCounter(api.PERF_CONN))
	assert.Equal(s.T(), 0.99, api.Statistics.GetCounter(api.PERF_UP_SPEED))
	assert.Equal(s.T(), 1.0, api.Statistics.GetCounter(api.PERF_DW_SPEED))
	assert.Equal(s.T(), 100.0, api.Statistics.GetCounter(api.PERF_UP_TOTAL))
	assert.Equal(s.T(), 10.0, api.Statistics.GetCounter(api.PERF_DW_TOTAL))
}

func (s *ClientSuite) TestClientStatisticsUpdate_Fail() {
	monkey.Patch(api.RequestStatistics, func(string, string, int, string) *api.StatsInfoResponse {
		return nil
	})

	ok := clientStatisticsUpdate("127.0.0.1", "127.0.0.1", 8080, "172.30.54.250")
	assert.False(s.T(), ok)
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
