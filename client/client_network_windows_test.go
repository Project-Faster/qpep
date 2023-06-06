//go:build windows

package client

import (
	"bou.ke/monkey"
	"bytes"
	"context"
	"fmt"
	"github.com/Project-Faster/quic-go"
	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/windivert"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
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
	shared.QPepConfig.MultiStream = false
	shared.QPepConfig.WinDivertThreads = 4
	shared.QPepConfig.PreferProxy = true
	shared.QPepConfig.Verbose = true
}

func (s *ClientNetworkSuite) AfterTest(_, testName string) {
	monkey.UnpatchAll()
	shared.SetSystemProxy(false)
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
	validateConfiguration()

	monkey.Patch(shared.SetSystemProxy, func(active bool) {
		assert.True(s.T(), active)
		shared.UsingProxy = true
		shared.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
	})

	assert.False(s.T(), shared.UsingProxy)
	initialCheckConnection()
	assert.True(s.T(), shared.UsingProxy)
	assert.NotNil(s.T(), shared.ProxyAddress)

	initialCheckConnection()
	assert.True(s.T(), shared.UsingProxy)
	assert.NotNil(s.T(), shared.ProxyAddress)
}

func (s *ClientNetworkSuite) TestFailedCheckConnection_PreferDiverterKeepRedirect() {
	var calledInit = false
	monkey.Patch(windivert.InitializeWinDivertEngine, func(string, string, int, int, int, int64) int {
		calledInit = true
		return windivert.DIVERT_OK
	})
	var calledStop = false
	monkey.Patch(windivert.CloseWinDivertEngine, func() int {
		calledStop = true
		return windivert.DIVERT_OK
	})

	shared.QPepConfig.PreferProxy = false
	validateConfiguration()
	keepRedirectionRetries = 15

	assert.False(s.T(), failedCheckConnection())

	assert.Equal(s.T(), 14, keepRedirectionRetries)
	assert.True(s.T(), calledInit)
	assert.True(s.T(), calledStop)
}

func (s *ClientNetworkSuite) TestFailedCheckConnection_PreferDiverterSwitchToProxy() {
	var calledInit = false
	monkey.Patch(windivert.InitializeWinDivertEngine, func(string, string, int, int, int, int64) int {
		calledInit = true
		return windivert.DIVERT_OK
	})
	var calledStop = false
	monkey.Patch(windivert.CloseWinDivertEngine, func() int {
		calledStop = true
		return windivert.DIVERT_OK
	})
	monkey.Patch(shared.SetSystemProxy, func(active bool) {
		assert.True(s.T(), active)
		shared.UsingProxy = true
		shared.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
	})

	shared.QPepConfig.PreferProxy = false
	validateConfiguration()
	keepRedirectionRetries = 2

	assert.False(s.T(), shared.UsingProxy)
	assert.False(s.T(), failedCheckConnection())

	assert.Equal(s.T(), 1, keepRedirectionRetries)
	assert.False(s.T(), calledInit)
	assert.True(s.T(), calledStop)

	assert.True(s.T(), shared.UsingProxy)
	assert.NotNil(s.T(), shared.ProxyAddress)
}

func (s *ClientNetworkSuite) TestFailedCheckConnection_PreferDiverterExhausted() {
	monkey.Patch(shared.SetSystemProxy, func(active bool) {
		assert.False(s.T(), active)
		shared.UsingProxy = false
		shared.ProxyAddress = nil
	})

	shared.QPepConfig.PreferProxy = false
	validateConfiguration()
	keepRedirectionRetries = 1

	shared.UsingProxy = true
	shared.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")

	assert.True(s.T(), failedCheckConnection())

	assert.Equal(s.T(), 0, keepRedirectionRetries)

	assert.False(s.T(), shared.UsingProxy)
	assert.Nil(s.T(), shared.ProxyAddress)
}

func (s *ClientNetworkSuite) TestFailedCheckConnection_PreferProxyKeepRedirect() {
	validateConfiguration()
	keepRedirectionRetries = 10
	shared.UsingProxy = true

	var callCounter = 0
	monkey.Patch(shared.SetSystemProxy, func(active bool) {
		assert.Equal(s.T(), shared.UsingProxy, active)
		if active {
			shared.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
		} else {
			shared.ProxyAddress = nil
		}
		callCounter++
	})

	assert.False(s.T(), failedCheckConnection())
	assert.Equal(s.T(), 1, callCounter)
	assert.Equal(s.T(), 9, keepRedirectionRetries)

	assert.True(s.T(), shared.UsingProxy)
	assert.NotNil(s.T(), shared.ProxyAddress)
}

func (s *ClientNetworkSuite) TestFailedCheckConnection_PreferProxySwitchToProxy_OK() {
	validateConfiguration()
	keepRedirectionRetries = 2
	shared.UsingProxy = true

	monkey.Patch(shared.SetSystemProxy, func(active bool) {
		assert.Equal(s.T(), shared.UsingProxy, active)
		if active {
			shared.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
		} else {
			shared.ProxyAddress = nil
		}
	})
	var calledInit = false
	monkey.Patch(windivert.InitializeWinDivertEngine, func(string, string, int, int, int, int64) int {
		calledInit = true
		return windivert.DIVERT_OK
	})

	assert.False(s.T(), failedCheckConnection())
	assert.Equal(s.T(), 1, keepRedirectionRetries)
	assert.True(s.T(), calledInit)

	assert.False(s.T(), shared.UsingProxy)
	assert.Nil(s.T(), shared.ProxyAddress)
}

func (s *ClientNetworkSuite) TestFailedCheckConnection_PreferProxySwitchToProxy_Fail() {
	validateConfiguration()
	keepRedirectionRetries = 2
	shared.UsingProxy = true

	monkey.Patch(shared.SetSystemProxy, func(active bool) {
		assert.Equal(s.T(), shared.UsingProxy, active)
		if active {
			shared.ProxyAddress, _ = url.Parse("http://127.0.0.1:8080")
		} else {
			shared.ProxyAddress = nil
		}
	})
	var calledInit = false
	monkey.Patch(windivert.InitializeWinDivertEngine, func(string, string, int, int, int, int64) int {
		calledInit = true
		return windivert.DIVERT_ERROR_FAILED
	})

	assert.True(s.T(), failedCheckConnection())
	assert.Equal(s.T(), 1, keepRedirectionRetries)
	assert.True(s.T(), calledInit)

	assert.False(s.T(), shared.UsingProxy)
	assert.Nil(s.T(), shared.ProxyAddress)
}

func (s *ClientNetworkSuite) TestFailedCheckConnection_PreferProxyExhausted() {
	validateConfiguration()
	keepRedirectionRetries = 1
	shared.UsingProxy = false

	var calledClose = false
	monkey.Patch(windivert.CloseWinDivertEngine, func() int {
		calledClose = true
		return windivert.DIVERT_OK
	})

	assert.True(s.T(), failedCheckConnection())
	assert.Equal(s.T(), 0, keepRedirectionRetries)
	assert.True(s.T(), calledClose)

	assert.False(s.T(), shared.UsingProxy)
	assert.Nil(s.T(), shared.ProxyAddress)
}

func (s *ClientNetworkSuite) TestOpenQuicSession() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	shared.QPepConfig.MaxConnectionRetries = 5
	validateConfiguration()

	wg := &sync.WaitGroup{}
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())

	go fakeQuicListener(ctx, cancel, s.T(), wg)

	conn, err := openQuicSession()
	assert.Nil(s.T(), err)

	quicStream, err := conn.OpenStreamSync(context.Background())
	assert.Nil(s.T(), err)

	sessionHeader := &shared.QPepHeader{
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

	assert.Equal(s.T(),
		"{\"SourceAddr\":{\"IP\":\"127.0.0.1\",\"Port\":0,\"Zone\":\"\"},\"DestAddr\":{\"IP\":\"172.50.20.100\",\"Port\":9999,\"Zone\":\"\"}}",
		string(buff[:n]))

	cancel()

	wg.Wait()
}

func (s *ClientNetworkSuite) TestOpenQuicSession_Fail() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	shared.QPepConfig.MaxConnectionRetries = 1
	validateConfiguration()

	conn, err := openQuicSession()
	assert.Equal(s.T(), shared.ErrFailedGatewayConnect, err)
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
	monkey.Patch(getQuicStream, func() (quic.Stream, error) {
		calledGetStream = true
		return &fakeStream{}, nil
	})
	var calledQuicHandler = false
	monkey.Patch(handleTcpToQuic, func(_ context.Context, wg *sync.WaitGroup, _ quic.Stream, _ net.Conn) {
		calledQuicHandler = true
		wg.Done()
	})
	var calledTcpHandler = false
	monkey.Patch(handleQuicToTcp, func(_ context.Context, wg *sync.WaitGroup, _ net.Conn, _ quic.Stream) {
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
	shared.QPepConfig.MultiStream = true
	validateConfiguration()

	fakeConn := &fakeTcpConn{}

	var calledGetStream = false
	monkey.Patch(getQuicStream, func() (quic.Stream, error) {
		calledGetStream = true
		quicSession = &fakeQuicConnection{}
		return &fakeStream{}, nil
	})
	var calledQuicHandler = false
	monkey.Patch(handleTcpToQuic, func(_ context.Context, wg *sync.WaitGroup, _ quic.Stream, _ net.Conn) {
		calledQuicHandler = true
		wg.Done()
	})
	var calledTcpHandler = false
	monkey.Patch(handleQuicToTcp, func(_ context.Context, wg *sync.WaitGroup, _ net.Conn, _ quic.Stream) {
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
	monkey.Patch(getQuicStream, func() (quic.Stream, error) {
		calledGetStream = true
		return nil, shared.ErrFailed
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
	monkey.Patch(getQuicStream, func() (quic.Stream, error) {
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
	monkey.Patch(handleTcpToQuic, func(_ context.Context, wg *sync.WaitGroup, _ quic.Stream, _ net.Conn) {
		calledQuicHandler = true
		wg.Done()
	})
	var calledTcpHandler = false
	monkey.Patch(handleQuicToTcp, func(_ context.Context, wg *sync.WaitGroup, _ net.Conn, _ quic.Stream) {
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
	ClientConfiguration.MultiStream = false
	quicSession = nil

	monkey.Patch(openQuicSession, func() (quic.Connection, error) {
		return &fakeQuicConnection{}, nil
	})

	stream, err := getQuicStream(context.Background())
	assert.Nil(s.T(), err)
	assert.NotNil(s.T(), stream)

	assert.NotNil(s.T(), quicSession)
}

func (s *ClientNetworkSuite) TestGetQuicStream_FailOpenSession() {
	ClientConfiguration.MultiStream = false
	quicSession = nil

	monkey.Patch(openQuicSession, func() (quic.Connection, error) {
		return nil, shared.ErrFailedGatewayConnect
	})

	stream, err := getQuicStream(context.Background())
	assert.Equal(s.T(), shared.ErrFailedGatewayConnect, err)
	assert.Nil(s.T(), stream)

	assert.Nil(s.T(), quicSession)
}

func (s *ClientNetworkSuite) TestGetQuicStream_MultiStream() {
	ClientConfiguration.MultiStream = true
	quicSession = nil

	monkey.Patch(openQuicSession, func() (quic.Connection, error) {
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

	go handleTcpToQuic(ctx, wg, dstConn, srcConn)

	wg.Wait()

	assert.NotNil(s.T(), dstConn.writtenData)
	assert.Equal(s.T(), len(testdata), dstConn.writtenData.Len())
}

func (s *ClientNetworkSuite) TestHandleQuicToTcp() {
	ctx, _ := context.WithCancel(context.Background())
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

	go handleQuicToTcp(ctx, wg, dstConn, srcConn)

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
		header := &shared.QPepHeader{
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
			assert.True(s.T(), strings.Contains(okResp, "200 OK"))
		} else {
			assert.True(s.T(), dstConn.writtenData.Len() >= len(headerBytes))
		}

		recvHeader, err := shared.QPepHeaderFromBytes(dstConn.writtenData)
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
	assert.Equal(s.T(), shared.ErrNonProxyableRequest, handleError)
}

func (s *ClientNetworkSuite) TestHandleProxyOpenConnection_FailHttpRead() {
	srcConn := &fakeTcpConn{}
	srcConn.readData = &bytes.Buffer{}
	var testdata = `GET `
	srcConn.readData.WriteString(testdata)

	request, handleError := handleProxyOpenConnection(srcConn)

	assert.Nil(s.T(), request)
	assert.Equal(s.T(), shared.ErrNonProxyableRequest, handleError)
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
	assert.Equal(s.T(), shared.ErrNonProxyableRequest, handleError)
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
	assert.Equal(s.T(), shared.ErrNonProxyableRequest, handleError)
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
	assert.Equal(s.T(), shared.ErrNonProxyableRequest, handleError)

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
	id          int
	closed      bool
	readData    *bytes.Buffer
	writtenData *bytes.Buffer
}

func (f *fakeStream) StreamID() quic.StreamID {
	return 0
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

func (f *fakeStream) CancelRead(code quic.StreamErrorCode) {}

func (f *fakeStream) SetReadDeadline(t time.Time) error {
	return nil
}

func (f *fakeStream) CancelWrite(code quic.StreamErrorCode) {}

func (f *fakeStream) Context() context.Context {
	return nil
}

func (f *fakeStream) SetWriteDeadline(t time.Time) error {
	return nil
}

func (f *fakeStream) SetDeadline(t time.Time) error {
	return nil
}

var _ quic.Stream = &fakeStream{}

// -------------------------- //

type fakeQuicConnection struct{}

func (f *fakeQuicConnection) AcceptStream(ctx context.Context) (quic.Stream, error) {
	return nil, nil
}

func (f *fakeQuicConnection) AcceptUniStream(ctx context.Context) (quic.ReceiveStream, error) {
	return nil, nil
}

func (f *fakeQuicConnection) OpenStream() (quic.Stream, error) {
	return &fakeStream{id: rand.Int()}, nil
}

func (f *fakeQuicConnection) OpenStreamSync(ctx context.Context) (quic.Stream, error) {
	return &fakeStream{id: rand.Int()}, nil
}

func (f *fakeQuicConnection) OpenUniStream() (quic.SendStream, error) {
	return nil, nil
}

func (f *fakeQuicConnection) OpenUniStreamSync(ctx context.Context) (quic.SendStream, error) {
	return nil, nil
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

func (f *fakeQuicConnection) CloseWithError(code quic.ApplicationErrorCode, s string) error {
	return nil
}

func (f *fakeQuicConnection) Context() context.Context {
	return context.Background()
}

func (f *fakeQuicConnection) ConnectionState() quic.ConnectionState {
	return quic.ConnectionState{}
}

func (f *fakeQuicConnection) SendMessage(i []byte) error {
	return nil
}

func (f *fakeQuicConnection) ReceiveMessage() ([]byte, error) {
	return nil, nil
}

var _ quic.Connection = &fakeQuicConnection{}
