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
	"github.com/lucas-clemente/quic-go"
	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/windivert"
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

func (s *ClientSuite) TestHandleServices() {
	proxyListener, _ = net.Listen("tcp", "127.0.0.1:9090")
	defer func() {
		if proxyListener != nil {
			_ = proxyListener.Close()
		}
	}()

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
		return true, &api.EchoResponse{
			Address:       "172.20.50.150",
			Port:          54635,
			ServerVersion: "0.1.0",
		}
	})
	var calledStatsUpdate = false
	monkey.Patch(clientStatisticsUpdate, func(_ string, _ string, _ int, pubAddress string) bool {
		assert.Equal(s.T(), "172.20.50.150", pubAddress)
		if !calledStatsUpdate {
			calledStatsUpdate = true
			wg2.Done()
		}
		return true
	})

	wg := &sync.WaitGroup{}
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	go handleServices(ctx, cancel, wg)

	wg2.Wait()
	cancel()
	wg.Wait()

	assert.True(s.T(), calledInitialCheck)
	assert.True(s.T(), calledGatewayCheck)
	assert.True(s.T(), calledStatsUpdate)
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

	wg2.Wait()
	cancel()
	wg.Wait()

	assert.True(s.T(), calledInitialCheck)
	assert.True(s.T(), calledGatewayCheck)
	assert.True(s.T(), calledFailedConnectionFirst)
	assert.True(s.T(), calledFailedConnectionSecond)
}

func (s *ClientSuite) TestHandleServices_FailStatistics() {
	wg2 := &sync.WaitGroup{}
	wg2.Add(4)
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
	var calledStatsUpdate = false
	monkey.Patch(clientStatisticsUpdate, func(_ string, _ string, _ int, pubAddress string) bool {
		assert.Equal(s.T(), "172.20.50.150", pubAddress)
		if !calledStatsUpdate {
			calledStatsUpdate = true
			wg2.Done()
		}
		return false
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

	wg2.Wait()
	cancel()
	wg.Wait()

	assert.True(s.T(), calledInitialCheck)
	assert.True(s.T(), calledGatewayCheck)
	assert.True(s.T(), calledStatsUpdate)
	assert.True(s.T(), calledFailedConnectionFirst)
	assert.True(s.T(), calledFailedConnectionSecond)
}

func (s *ClientSuite) TestInitProxy() {
	monkey.Patch(shared.SetSystemProxy, func(active bool) {
		assert.True(s.T(), active)
		shared.UsingProxy = true
		shared.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
	})

	assert.False(s.T(), shared.UsingProxy)
	initProxy()
	assert.True(s.T(), shared.UsingProxy)

	assert.NotNil(s.T(), shared.ProxyAddress)
}

func (s *ClientSuite) TestStopProxy() {
	shared.UsingProxy = true
	shared.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")

	monkey.Patch(shared.SetSystemProxy, func(active bool) {
		assert.False(s.T(), active)
		shared.UsingProxy = false
		shared.ProxyAddress = nil
	})

	assert.True(s.T(), shared.UsingProxy)
	stopProxy()
	assert.False(s.T(), shared.UsingProxy)
	assert.False(s.T(), redirected)

	assert.Nil(s.T(), shared.ProxyAddress)
}

func (s *ClientSuite) TestInitDiverter() {
	if runtime.GOOS != "windows" {
		assert.False(s.T(), initDiverter())
		return
	}
	monkey.Patch(windivert.InitializeWinDivertEngine, func(string, string, int, int, int, int64) int {
		return windivert.DIVERT_OK
	})
	assert.True(s.T(), initDiverter())
}

func (s *ClientSuite) TestInitDiverter_Fail() {
	if runtime.GOOS != "windows" {
		assert.False(s.T(), initDiverter())
		return
	}
	monkey.Patch(windivert.InitializeWinDivertEngine, func(string, string, int, int, int, int64) int {
		return windivert.DIVERT_ERROR_ALREADY_INIT
	})
	assert.False(s.T(), initDiverter())
}

func (s *ClientSuite) TestStopDiverter() {
	if runtime.GOOS != "windows" {
		assert.False(s.T(), initDiverter())
		return
	}
	monkey.Patch(windivert.CloseWinDivertEngine, func() int {
		return windivert.DIVERT_OK
	})
	stopDiverter()
	assert.False(s.T(), redirected)
}

func (s *ClientSuite) TestInitialCheckConnection() {
	shared.QPepConfig.PreferProxy = false
	validateConfiguration()

	monkey.Patch(shared.SetSystemProxy, func(active bool) {
		assert.True(s.T(), active)
		shared.UsingProxy = true
		shared.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
	})

	assert.False(s.T(), shared.UsingProxy)
	initialCheckConnection()
	assert.False(s.T(), shared.UsingProxy)
	assert.Nil(s.T(), shared.ProxyAddress)

	initialCheckConnection()
	assert.False(s.T(), shared.UsingProxy)
	assert.Nil(s.T(), shared.ProxyAddress)
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
	quicClientConfig := shared.GetQuicConfiguration()

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
