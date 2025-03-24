//go:build linux

package client

import (
	"bytes"
	"context"
	"fmt"
	"github.com/Project-Faster/monkey"
	"github.com/Project-Faster/qpep/api"
	"github.com/Project-Faster/qpep/backend"
	"github.com/Project-Faster/qpep/shared/configuration"
	"github.com/Project-Faster/qpep/shared/errors"
	"github.com/Project-Faster/qpep/shared/logger"
	"github.com/Project-Faster/qpep/shared/protocol"
	"github.com/Project-Faster/qpep/windivert"
	"github.com/Project-Faster/qpep/workers/gateway"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientNetworkSuite(t *testing.T) {
	var q ClientNetworkSuite
	suite.Run(t, &q)
}

type ClientNetworkSuite struct {
	suite.Suite

	mtx sync.Mutex
}

func (s *ClientNetworkSuite) BeforeTest(_, testName string) {
	gateway.ResetScaleTimeout()

	api.Statistics.Reset()
	proxyListener = nil

	redirected = false
	gateway.UsingProxy = false
	gateway.ProxyAddress = nil

	backend.GenerateTLSConfig("cert.pem", "key.pem")

	configuration.QPepConfig = configuration.QPepConfigType{}
	configuration.QPepConfig.Merge(&configuration.DefaultConfig)

	configuration.QPepConfig.Security.Certificate = "cert.pem"
	configuration.QPepConfig.Security.PrivateKey = "key.pem"

	configuration.QPepConfig.Client.GatewayHost = "127.0.0.2"
	configuration.QPepConfig.Client.GatewayPort = 9443
	configuration.QPepConfig.Client.LocalListeningAddress = "127.0.0.1"
	configuration.QPepConfig.Client.LocalListenPort = 9090

	configuration.QPepConfig.Protocol.BufferSize = 32

	configuration.QPepConfig.General.APIPort = 445
	configuration.QPepConfig.General.MaxConnectionRetries = 15
	configuration.QPepConfig.General.MultiStream = false
	configuration.QPepConfig.General.WinDivertThreads = 4
	configuration.QPepConfig.General.PreferProxy = true
	configuration.QPepConfig.General.Verbose = false

	logger.SetupLogger(testName, "info", false)
}

func (s *ClientNetworkSuite) AfterTest(_, testName string) {
	monkey.UnpatchAll()
	gateway.SetSystemProxy(false)
}

func (s *ClientNetworkSuite) TestGetAddressPortFromHost() {
	netIP := net.ParseIP("127.0.0.1")
	for _, val := range []string{"", ":8080", "127.0.0.1", "127.0.0.1:8080"} {
		addr, port, valid := getAddressPortFromHost(val)
		assert.True(s.T(), valid)
		assert.Equal(s.T(), netIP.String(), addr.String())
		if strings.Contains(val, "8080") {
			assert.Equal(s.T(), 8080, port)
		}
	}
}

func (s *ClientNetworkSuite) TestGetAddressPortFromHost_Invalid() {
	for _, val := range []string{":", ":TEST", "1:2:3"} {
		_, _, valid := getAddressPortFromHost(val)
		assert.False(s.T(), valid)
	}
}

func (s *ClientNetworkSuite) TestInitialCheckConnection_PreferProxy() {
	s.T().Skipf("Proxy set not supported on linux")
	return

	validateConfiguration()

	monkey.Patch(gateway.SetSystemProxy, func(active bool) {
		assert.True(s.T(), active)
		gateway.UsingProxy = true
		gateway.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
	})

	assert.False(s.T(), gateway.UsingProxy)
	initialCheckConnection()
	assert.True(s.T(), gateway.UsingProxy)
	assert.NotNil(s.T(), gateway.ProxyAddress)

	initialCheckConnection()
	assert.True(s.T(), gateway.UsingProxy)
	assert.NotNil(s.T(), gateway.ProxyAddress)
}

func (s *ClientNetworkSuite) TestFailedCheckConnection_PreferDiverterKeepRedirect() {
	s.T().Skipf("Proxy set not supported on linux")
	return

	var calledInit = false
	monkey.Patch(windivert.InitializeWinDivertEngine, func(string, string, int, int, int, int64, []int) int {
		calledInit = true
		return windivert.DIVERT_OK
	})
	var calledStop = false
	monkey.Patch(windivert.CloseWinDivertEngine, func() int {
		calledStop = true
		return windivert.DIVERT_OK
	})

	configuration.QPepConfig.General.PreferProxy = false
	validateConfiguration()
	keepRedirectionRetries = 15

	assert.False(s.T(), failedCheckConnection())

	assert.Equal(s.T(), 14, keepRedirectionRetries)
	assert.True(s.T(), calledInit)
	assert.False(s.T(), calledStop)
}

func (s *ClientNetworkSuite) TestFailedCheckConnection_PreferDiverterSwitchToProxy() {
	s.T().Skipf("Proxy set not supported on linux")
	return

	var calledInit = false
	monkey.Patch(initDiverter, func() bool {
		calledInit = true
		return true
	})
	var calledStop = false
	monkey.Patch(stopDiverter, func() {
		calledStop = true
	})
	monkey.Patch(initProxy, func() {
		gateway.UsingProxy = true
		gateway.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
	})

	configuration.QPepConfig.General.PreferProxy = false
	validateConfiguration()
	keepRedirectionRetries = 2

	assert.False(s.T(), gateway.UsingProxy)
	assert.False(s.T(), failedCheckConnection())

	assert.Equal(s.T(), 1, keepRedirectionRetries)
	assert.False(s.T(), calledInit)
	assert.True(s.T(), calledStop)

	assert.True(s.T(), gateway.UsingProxy)
	assert.NotNil(s.T(), gateway.ProxyAddress)
}

func (s *ClientNetworkSuite) TestFailedCheckConnection_PreferDiverterExhausted() {
	s.T().Skipf("Proxy set not supported on linux")
	return

	monkey.Patch(gateway.SetSystemProxy, func(active bool) {
		assert.False(s.T(), active)
		gateway.UsingProxy = false
		gateway.ProxyAddress = nil
	})

	configuration.QPepConfig.General.PreferProxy = false
	validateConfiguration()
	keepRedirectionRetries = 1

	gateway.UsingProxy = true
	gateway.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")

	assert.True(s.T(), failedCheckConnection())

	assert.Equal(s.T(), 0, keepRedirectionRetries)

	assert.False(s.T(), gateway.UsingProxy)
	assert.Nil(s.T(), gateway.ProxyAddress)
}

func (s *ClientNetworkSuite) TestFailedCheckConnection_PreferProxyKeepRedirect() {
	s.T().Skipf("Proxy set not supported on linux")
	return

	validateConfiguration()
	keepRedirectionRetries = 10
	gateway.UsingProxy = true

	var callCounter = 0
	monkey.Patch(gateway.SetSystemProxy, func(active bool) {
		assert.Equal(s.T(), gateway.UsingProxy, active)
		if active {
			gateway.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
		} else {
			gateway.ProxyAddress = nil
		}
		callCounter++
	})

	assert.False(s.T(), failedCheckConnection())
	assert.Equal(s.T(), 1, callCounter)
	assert.Equal(s.T(), 9, keepRedirectionRetries)

	assert.True(s.T(), gateway.UsingProxy)
	assert.NotNil(s.T(), gateway.ProxyAddress)
}

func (s *ClientNetworkSuite) TestFailedCheckConnection_PreferProxySwitchToProxy_OK() {
	s.T().Skipf("Proxy set not supported on linux")
	return

	validateConfiguration()
	keepRedirectionRetries = 2
	gateway.UsingProxy = true

	monkey.Patch(gateway.SetSystemProxy, func(active bool) {
		assert.Equal(s.T(), gateway.UsingProxy, active)
		if active {
			gateway.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
		} else {
			gateway.ProxyAddress = nil
		}
	})
	var calledInit = false
	monkey.Patch(windivert.InitializeWinDivertEngine, func(string, string, int, int, int, int64, []int) int {
		calledInit = true
		return windivert.DIVERT_OK
	})

	assert.False(s.T(), failedCheckConnection())
	assert.Equal(s.T(), 1, keepRedirectionRetries)
	assert.True(s.T(), calledInit)

	assert.False(s.T(), gateway.UsingProxy)
	assert.Nil(s.T(), gateway.ProxyAddress)
}

func (s *ClientNetworkSuite) TestFailedCheckConnection_PreferProxySwitchToProxy_Fail() {
	s.T().Skipf("Proxy set not supported on linux")
	return

	validateConfiguration()
	keepRedirectionRetries = 2
	gateway.UsingProxy = true

	monkey.Patch(gateway.SetSystemProxy, func(active bool) {
		assert.Equal(s.T(), gateway.UsingProxy, active)
		if active {
			gateway.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
		} else {
			gateway.ProxyAddress = nil
		}
	})
	var calledInit = false
	monkey.Patch(windivert.InitializeWinDivertEngine, func(string, string, int, int, int, int64, []int) int {
		calledInit = true
		return windivert.DIVERT_ERROR_FAILED
	})

	assert.True(s.T(), failedCheckConnection())
	assert.Equal(s.T(), 1, keepRedirectionRetries)
	assert.True(s.T(), calledInit)

	assert.False(s.T(), gateway.UsingProxy)
	assert.Nil(s.T(), gateway.ProxyAddress)
}

func (s *ClientNetworkSuite) TestFailedCheckConnection_PreferProxyExhausted() {
	s.T().Skipf("Proxy set not supported on linux")
	return

	validateConfiguration()
	keepRedirectionRetries = 1
	gateway.UsingProxy = false

	var calledClose = false
	monkey.Patch(stopDiverter, func() {
		calledClose = true
	})

	assert.True(s.T(), failedCheckConnection())
	assert.Equal(s.T(), 0, keepRedirectionRetries)
	assert.True(s.T(), calledClose)

	assert.False(s.T(), gateway.UsingProxy)
	assert.Nil(s.T(), gateway.ProxyAddress)
}

func (s *ClientNetworkSuite) TestOpenQuicSession() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	configuration.QPepConfig.General.MaxConnectionRetries = 5
	validateConfiguration()

	wg := &sync.WaitGroup{}
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())

	go fakeQuicListener(ctx, cancel, s.T(), wg)

	conn, err := openQuicSession()
	assert.Nil(s.T(), err)

	quicStream, err := conn.OpenStream(context.Background())
	assert.Nil(s.T(), err)

	sessionHeader := &protocol.QPepHeader{
		SourceAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0},
		DestAddr:   &net.TCPAddr{IP: net.ParseIP("172.50.20.100"), Port: 9999},
	}

	quicStream.SetWriteDeadline(time.Now().Add(1 * time.Second))
	_, _ = quicStream.Write(sessionHeader.ToBytes())

	quicStream.SetReadDeadline(time.Now().Add(1 * time.Second))

	buff := make([]byte, 1024)
	n, err := quicStream.Read(buff)
	if err != nil && err != io.EOF {
		s.T().Fatalf("Quic read error not nil or EOF")
	}

	cancel()
	wg.Wait()

	assert.Equal(s.T(),
		"{\"SourceAddr\":{\"IP\":\"127.0.0.1\",\"Port\":0,\"Zone\":\"\"},\"DestAddr\":{\"IP\":\"172.50.20.100\",\"Port\":9999,\"Zone\":\"\"},\"Flags\":0}",
		string(buff[:n]))
}

func (s *ClientNetworkSuite) TestOpenQuicSession_Fail() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	configuration.QPepConfig.General.MaxConnectionRetries = 1
	validateConfiguration()

	conn, err := openQuicSession()
	assert.Equal(s.T(), errors.ErrFailedGatewayConnect, err)
	assert.Nil(s.T(), conn)

	<-time.After(1 * time.Second)
}

func (s *ClientNetworkSuite) TestListenTCPConn() {
	proxyListener, _ = net.Listen("tcp", "127.0.0.1:9090")
	defer func() {
		_ = proxyListener.Close()
	}()

	wg := &sync.WaitGroup{}
	wg.Add(2)

	var calledHandle = false
	monkey.Patch(handleTCPConn, func(conn net.Conn) {
		defer wg.Done()
		calledHandle = true
		assert.NotNil(s.T(), conn)
	})

	go func() {
		assert.NotPanics(s.T(), func() {
			listenTCPConn(wg)
		})
	}()

	<-time.After(1 * time.Second)

	conn, err := net.DialTCP("tcp", nil, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9090})
	assert.Nil(s.T(), err)
	assert.NotNil(s.T(), conn)

	conn.Close()

	proxyListener.Close()

	wg.Wait()

	assert.True(s.T(), calledHandle)
}

func (s *ClientNetworkSuite) TestListenTCPConn_PanicError() {
	proxyListener = nil

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		assert.NotPanics(s.T(), func() {
			listenTCPConn(wg)
		})
	}()

	wg.Wait()
}

func (s *ClientNetworkSuite) TestHandleTCPConn_NoMultistream() {
	validateConfiguration()

	fakeConn := &fakeTcpConn{}

	var calledGetStream = false
	monkey.Patch(getQuicStream, func(_ context.Context) (backend.QuicBackendStream, error) {
		calledGetStream = true
		return &fakeStream{}, nil
	})
	var calledQuicHandler = false
	monkey.Patch(handleTcpToQuic, func(_ context.Context, wg *sync.WaitGroup, _ backend.QuicBackendStream, _ net.Conn, _, _ *atomic.Bool) {
		calledQuicHandler = true
		wg.Done()
	})
	var calledTcpHandler = false
	monkey.Patch(handleQuicToTcp, func(_ context.Context, wg *sync.WaitGroup, _ net.Conn, _ backend.QuicBackendStream, _, _ *atomic.Bool) {
		calledTcpHandler = true
		wg.Done()
	})

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		handleTCPConn(fakeConn)
	}()

	wg.Wait()

	assert.True(s.T(), calledGetStream)
	assert.True(s.T(), calledQuicHandler)
	assert.True(s.T(), calledTcpHandler)

	assert.Nil(s.T(), quicSession)
}

func (s *ClientNetworkSuite) TestHandleTCPConn_Multistream() {
	configuration.QPepConfig.General.MultiStream = true
	validateConfiguration()

	fakeConn := &fakeTcpConn{}

	var calledGetStream = false
	monkey.Patch(getQuicStream, func(_ context.Context) (backend.QuicBackendStream, error) {
		calledGetStream = true
		quicSession = &fakeQuicConnection{}
		return &fakeStream{}, nil
	})
	var calledQuicHandler = false
	monkey.Patch(handleTcpToQuic, func(_ context.Context, wg *sync.WaitGroup, _ backend.QuicBackendStream, _ net.Conn, _, _ *atomic.Bool) {
		calledQuicHandler = true
		wg.Done()
	})
	var calledTcpHandler = false
	monkey.Patch(handleQuicToTcp, func(_ context.Context, wg *sync.WaitGroup, _ net.Conn, _ backend.QuicBackendStream, _, _ *atomic.Bool) {
		calledTcpHandler = true
		wg.Done()
	})

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		handleTCPConn(fakeConn)
	}()

	wg.Wait()

	assert.True(s.T(), calledGetStream)
	assert.True(s.T(), calledQuicHandler)
	assert.True(s.T(), calledTcpHandler)

	assert.NotNil(s.T(), quicSession)
}

func (s *ClientNetworkSuite) TestHandleTCPConn_PanicError() {
	assert.NotPanics(s.T(), func() {
		handleTCPConn(nil)
	})
}

func (s *ClientNetworkSuite) TestHandleTCPConn_FailGetStream() {
	validateConfiguration()
	quicSession = nil

	fakeConn := &fakeTcpConn{}

	var calledGetStream = false
	monkey.Patch(getQuicStream, func(_ context.Context) (backend.QuicBackendStream, error) {
		calledGetStream = true
		return nil, errors.ErrFailed
	})

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		handleTCPConn(fakeConn)
	}()

	wg.Wait()

	assert.True(s.T(), calledGetStream)
	assert.Nil(s.T(), quicSession)

	assert.True(s.T(), fakeConn.closed)
}

func (s *ClientNetworkSuite) TestHandleTCPConn_NoMultistreamProxy() {
	validateConfiguration()

	fakeConn := &fakeTcpConn{}

	var calledGetStream = false
	monkey.Patch(getQuicStream, func(_ context.Context) (backend.QuicBackendStream, error) {
		calledGetStream = true
		return &fakeStream{}, nil
	})
	monkey.Patch(windivert.GetConnectionStateData, func(port int) (int, int, int, string, string) {
		return windivert.DIVERT_ERROR_FAILED, 0, 0, "", ""
	})
	var calledHandleProxy = false
	monkey.Patch(handleProxyOpenConnection, func(net.Conn) (*http.Request, error) {
		calledHandleProxy = true
		return nil, nil
	})
	var calledQuicHandler = false
	monkey.Patch(handleTcpToQuic, func(_ context.Context, wg *sync.WaitGroup, _ backend.QuicBackendStream, _ net.Conn, _, _ *atomic.Bool) {
		calledQuicHandler = true
		wg.Done()
	})
	var calledTcpHandler = false
	monkey.Patch(handleQuicToTcp, func(_ context.Context, wg *sync.WaitGroup, _ net.Conn, _ backend.QuicBackendStream, _, _ *atomic.Bool) {
		calledTcpHandler = true
		wg.Done()
	})

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		handleTCPConn(fakeConn)
	}()

	wg.Wait()

	assert.True(s.T(), calledGetStream)
	assert.True(s.T(), calledHandleProxy)
	assert.True(s.T(), calledQuicHandler)
	assert.True(s.T(), calledTcpHandler)

	assert.Nil(s.T(), quicSession)
}

func (s *ClientNetworkSuite) TestGetQuicStream() {
	configuration.QPepConfig.General.MultiStream = false
	quicSession = nil

	monkey.Patch(openQuicSession, func() (backend.QuicBackendConnection, error) {
		return &fakeQuicConnection{}, nil
	})

	stream, err := getQuicStream(context.Background())
	assert.Nil(s.T(), err)
	assert.NotNil(s.T(), stream)

	assert.NotNil(s.T(), quicSession)
}

func (s *ClientNetworkSuite) TestGetQuicStream_FailOpenSession() {
	configuration.QPepConfig.General.MultiStream = false
	quicSession = nil

	monkey.Patch(openQuicSession, func() (backend.QuicBackendConnection, error) {
		return nil, errors.ErrFailedGatewayConnect
	})

	stream, err := getQuicStream(context.Background())
	assert.Equal(s.T(), errors.ErrFailedGatewayConnect, err)
	assert.Nil(s.T(), stream)

	assert.Nil(s.T(), quicSession)
}

func (s *ClientNetworkSuite) TestGetQuicStream_MultiStream() {
	configuration.QPepConfig.General.MultiStream = true
	quicSession = nil

	monkey.Patch(openQuicSession, func() (backend.QuicBackendConnection, error) {
		return &fakeQuicConnection{}, nil
	})

	stream, err := getQuicStream(context.Background())
	assert.Nil(s.T(), err)
	assert.NotNil(s.T(), stream)

	assert.NotNil(s.T(), quicSession)

	// second stream
	stream2, err2 := getQuicStream(context.Background())
	assert.Nil(s.T(), err2)
	assert.NotNil(s.T(), stream2)

	assert.NotNil(s.T(), quicSession)

	assert.NotEqual(s.T(), stream, stream2)
}

func (s *ClientNetworkSuite) TestHandleTcpToQuic() {
	ctx, _ := context.WithCancel(context.Background())

	var qtFlag, tqFlag atomic.Bool
	qtFlag.Store(true)
	tqFlag.Store(true)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	dstConn := &fakeStream{}
	srcConn := &fakeTcpConn{}

	srcConn.readData = &bytes.Buffer{}
	const testdata = `GET /api/v1/server/echo HTTP/1.1
Host: :9443
Accept: application/json
Accept-Encoding: gzip
User-Agent: windows

`
	srcConn.readData.WriteString(testdata)

	go handleTcpToQuic(ctx, wg, dstConn, srcConn, &qtFlag, &tqFlag)

	wg.Wait()

	assert.NotNil(s.T(), dstConn.writtenData)
	assert.Equal(s.T(), len(testdata), dstConn.writtenData.Len())
}

func (s *ClientNetworkSuite) TestHandleQuicToTcp() {
	ctx, _ := context.WithCancel(context.Background())

	var qtFlag, tqFlag atomic.Bool
	qtFlag.Store(true)
	tqFlag.Store(true)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	srcConn := &fakeStream{}
	dstConn := &fakeTcpConn{}

	srcConn.readData = &bytes.Buffer{}
	const testdata = `GET /api/v1/server/echo HTTP/1.1
Host: :9443
Accept: application/json
Accept-Encoding: gzip
User-Agent: windows

`
	srcConn.readData.WriteString(testdata)

	go handleQuicToTcp(ctx, wg, dstConn, srcConn, &qtFlag, &tqFlag)

	wg.Wait()

	assert.NotNil(s.T(), dstConn.writtenData)
	assert.Equal(s.T(), len(testdata), dstConn.writtenData.Len())
}

var httpMethods = []string{
	http.MethodGet, http.MethodHead, http.MethodPost, http.MethodDelete,
	http.MethodPut, http.MethodPatch, http.MethodConnect, http.MethodOptions,
}

func (s *ClientNetworkSuite) TestHandleProxyOpenConnection() {
	srcIp := net.ParseIP("127.0.0.1")
	for _, method := range httpMethods {
		header := &protocol.QPepHeader{
			SourceAddr: &net.TCPAddr{
				IP:   srcIp,
				Port: 50000 + rand.Intn(10000),
			},
		}

		dstConn := &fakeStream{}
		srcConn := &fakeTcpConn{}

		srcConn.readData = &bytes.Buffer{}
		var testdata = fmt.Sprintf(`%s /api/v1/server/echo HTTP/1.1
Host: 192.168.1.100:9443
Accept: application/json
Accept-Encoding: gzip
User-Agent: windows


`, method)
		srcConn.readData.WriteString(testdata)

		var request, openError = handleProxyOpenConnection(srcConn)
		assert.Nil(s.T(), openError)

		assert.NotNil(s.T(), request)
		var handleError = handleProxyedRequest(request, header, srcConn, dstConn)

		assert.Nil(s.T(), handleError)
		assert.NotNil(s.T(), dstConn.writtenData)

		headerBytes := header.ToBytes()
		s.T().Logf("%v", headerBytes)
		s.T().Logf("%v", dstConn.writtenData.Bytes())
		s.T().Logf("%v", []byte(testdata))
		s.T().Logf("------------------")

		if method == "CONNECT" {
			assert.Equal(s.T(), len(headerBytes), dstConn.writtenData.Len())
			okResp := srcConn.writtenData.String()
			assert.True(s.T(), strings.Contains(okResp, "200 Connection established"))
		} else {
			assert.True(s.T(), dstConn.writtenData.Len() >= len(headerBytes))
		}

		var proxyWritten = &fakeStream{
			readData: dstConn.writtenData,
		}

		recvHeader, err := protocol.QPepHeaderFromBytes(proxyWritten)
		assert.Nil(s.T(), err)

		assert.NotNil(s.T(), recvHeader)
		assert.NotNil(s.T(), recvHeader.SourceAddr)
		assert.NotNil(s.T(), recvHeader.DestAddr)

		assert.Equal(s.T(), "127.0.0.1", recvHeader.SourceAddr.IP.String())
		assert.Equal(s.T(), header.SourceAddr.Port, recvHeader.SourceAddr.Port)
		assert.Equal(s.T(), "192.168.1.100", recvHeader.DestAddr.IP.String())
		assert.Equal(s.T(), 9443, recvHeader.DestAddr.Port)

		assert.False(s.T(), srcConn.closed)
		assert.False(s.T(), dstConn.closed)

		s.T().Logf("method %s - %v", method, !s.T().Failed())
		if s.T().Failed() {
			return
		}
	}
}

func (s *ClientNetworkSuite) TestHandleProxyOpenConnection_NoData() {
	srcConn := &fakeTcpConn{}

	request, handleError := handleProxyOpenConnection(srcConn)

	assert.Nil(s.T(), request)
	assert.Equal(s.T(), errors.ErrNonProxyableRequest, handleError)
}

func (s *ClientNetworkSuite) TestHandleProxyOpenConnection_FailHttpRead() {
	srcConn := &fakeTcpConn{}
	srcConn.readData = &bytes.Buffer{}
	var testdata = `GET `
	srcConn.readData.WriteString(testdata)

	request, handleError := handleProxyOpenConnection(srcConn)

	assert.Nil(s.T(), request)
	assert.Equal(s.T(), errors.ErrNonProxyableRequest, handleError)
}

func (s *ClientNetworkSuite) TestHandleProxyOpenConnection_FailHostRead_GET() {
	srcConn := &fakeTcpConn{}
	srcConn.readData = &bytes.Buffer{}
	var testdata = `GET /api/v1/server/echo HTTP/1.1
Host: TEST:9443

`
	srcConn.readData.WriteString(testdata)
	request, handleError := handleProxyOpenConnection(srcConn)

	assert.Nil(s.T(), request)
	assert.Equal(s.T(), errors.ErrNonProxyableRequest, handleError)
}

func (s *ClientNetworkSuite) TestHandleProxyOpenConnection_FailHostRead_CONNECT() {
	srcConn := &fakeTcpConn{}
	srcConn.readData = &bytes.Buffer{}
	var testdata = `CONNECT /api/v1/server/echo HTTP/1.1
Host: TEST:9443

`
	srcConn.readData.WriteString(testdata)
	request, handleError := handleProxyOpenConnection(srcConn)

	assert.Nil(s.T(), request)
	assert.Equal(s.T(), errors.ErrNonProxyableRequest, handleError)
}

func (s *ClientNetworkSuite) TestHandleProxyOpenConnection_FailUnrecognizedMethod() {
	dstConn := &fakeStream{}
	srcConn := &fakeTcpConn{}

	srcConn.readData = &bytes.Buffer{}
	var testdata = `UNKNOWN /api/v1/server/echo HTTP/1.1
Host: 192.168.1.100:9443
Accept: application/json
Accept-Encoding: gzip
User-Agent: windows


`

	srcConn.readData.WriteString(testdata)

	request, handleError := handleProxyOpenConnection(srcConn)

	assert.Nil(s.T(), request)
	assert.Equal(s.T(), errors.ErrNonProxyableRequest, handleError)

	assert.NotNil(s.T(), srcConn.writtenData)
	assert.True(s.T(), srcConn.writtenData.Len() > 0)
	badGatewayResp := srcConn.writtenData.String()
	assert.True(s.T(), strings.Contains(badGatewayResp, "502 Bad Gateway"))

	assert.True(s.T(), srcConn.closed)
	assert.False(s.T(), dstConn.closed)
}

// --- utilities --- //

type fakeTcpConn struct {
	closed      bool
	readData    *bytes.Buffer
	writtenData *bytes.Buffer
}

func (f *fakeTcpConn) Read(b []byte) (n int, err error) {
	if f.closed {
		panic("Read from closed connection")
	}
	if f.readData == nil {
		f.readData = &bytes.Buffer{}
	}
	return f.readData.Read(b)
}

func (f *fakeTcpConn) Write(b []byte) (n int, err error) {
	if f.closed {
		panic("Write to closed connection")
	}
	if f.writtenData == nil {
		f.writtenData = &bytes.Buffer{}
	}
	return f.writtenData.Write(b)
}

func (f *fakeTcpConn) Close() error {
	f.closed = true
	return nil
}

func (f *fakeTcpConn) LocalAddr() net.Addr {
	return &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 55555,
	}
}

func (f *fakeTcpConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{
		IP:   net.ParseIP("192.168.1.100"),
		Port: 9090,
	}
}

func (f *fakeTcpConn) SetDeadline(_ time.Time) error {
	return nil
}

func (f *fakeTcpConn) SetReadDeadline(_ time.Time) error {
	return nil
}

func (f *fakeTcpConn) SetWriteDeadline(_ time.Time) error {
	return nil
}

var _ net.Conn = &fakeTcpConn{}

// --------------------------- //

type fakeStream struct {
	id          uint64
	closed      bool
	readData    *bytes.Buffer
	writtenData *bytes.Buffer
}

func (f *fakeStream) ID() uint64 {
	return f.id
}

func (f *fakeStream) Sync() bool {
	return f.readData.Len() == f.writtenData.Len()
}

func (f *fakeStream) AbortRead(code uint64) {}

func (f *fakeStream) AbortWrite(code uint64) {}

func (f *fakeStream) IsClosed() bool {
	return f.closed
}

func (f *fakeStream) Read(b []byte) (n int, err error) {
	if f.closed {
		panic("Read from closed connection")
	}
	if f.readData == nil {
		f.readData = &bytes.Buffer{}
	}
	return f.readData.Read(b)
}

func (f *fakeStream) Write(b []byte) (n int, err error) {
	if f.closed {
		panic("Write to closed connection")
	}
	if f.writtenData == nil {
		f.writtenData = &bytes.Buffer{}
	}
	return f.writtenData.Write(b)
}

func (f *fakeStream) Close() error {
	f.closed = true
	return nil
}

func (f *fakeStream) SetReadDeadline(t time.Time) error {
	return nil
}

func (f *fakeStream) SetWriteDeadline(t time.Time) error {
	return nil
}

var _ backend.QuicBackendStream = &fakeStream{}

// -------------------------- //

type fakeQuicConnection struct {
	closeCalled bool
}

func (f *fakeQuicConnection) OpenStream(ctx context.Context) (backend.QuicBackendStream, error) {
	return &fakeStream{id: rand.Uint64()}, nil
}

func (f *fakeQuicConnection) AcceptStream(ctx context.Context) (backend.QuicBackendStream, error) {
	return nil, nil
}

func (f *fakeQuicConnection) AcceptConnection(ctx context.Context) (backend.QuicBackendConnection, error) {
	return nil, nil
}

func (f *fakeQuicConnection) Close(code int, message string) error {
	f.closeCalled = true
	return nil
}

func (f *fakeQuicConnection) IsClosed() bool {
	return f.closeCalled
}

func (f *fakeQuicConnection) LocalAddr() net.Addr {
	return &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 55555,
	}
}

func (f *fakeQuicConnection) RemoteAddr() net.Addr {
	return &net.TCPAddr{
		IP:   net.ParseIP("192.168.1.100"),
		Port: 9090,
	}
}

func (f *fakeQuicConnection) Context() context.Context {
	return context.Background()
}

var _ backend.QuicBackendConnection = &fakeQuicConnection{}
