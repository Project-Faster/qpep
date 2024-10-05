package server

import (
	"bufio"
	"context"
	"fmt"
	"github.com/Project-Faster/monkey"
	"github.com/Project-Faster/qpep/api"
	"github.com/Project-Faster/qpep/backend"
	"github.com/Project-Faster/qpep/shared/configuration"
	stderr "github.com/Project-Faster/qpep/shared/errors"
	"github.com/Project-Faster/qpep/shared/logger"
	"github.com/Project-Faster/qpep/shared/protocol"
	"github.com/Project-Faster/qpep/workers/gateway"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/cors"
	log "github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

var testlog log.Logger
var fakeBackend backend.QuicBackend = &testBackend{}

func TestServerSuite(t *testing.T) {
	_logFile, err := os.OpenFile("./speedtests.log", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	assert.Nil(t, err)

	testlog = log.New(_logFile).Level(log.DebugLevel).
		With().Timestamp().Logger()

	defer func() {
		_logFile.Close()
	}()

	var q ServerSuite
	suite.Run(t, &q)
}

type ServerSuite struct {
	suite.Suite
}

func (s *ServerSuite) BeforeTest(_, testName string) {
	backend.GenerateTLSConfig("cert.pem", "key.pem")

	configuration.QPepConfig = configuration.QPepConfigType{}
	configuration.QPepConfig.Merge(&configuration.DefaultConfig)

	configuration.QPepConfig.Server.LocalListeningAddress = "127.0.0.1"
	configuration.QPepConfig.Server.LocalListenPort = 9090
	configuration.QPepConfig.Security.Certificate = "cert.pem"
	configuration.QPepConfig.Security.PrivateKey = "key.pem"
	configuration.QPepConfig.Protocol.BufferSize = 32
	configuration.QPepConfig.General.APIPort = 9443

	logger.SetupLogger(testName, "info")
}

func (s *ServerSuite) AfterTest(_, testName string) {
	monkey.UnpatchAll()
	api.Statistics.Reset()
}

func (s *ServerSuite) TestPerformanceWatcher() {
	ctx, cancel := newContext()

	api.Statistics.SetMappedAddress("127.0.0.1", "192.168.1.100")

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		performanceWatcher(ctx, cancel)
	}()

	for i := 0; i < 3; i++ {
		api.Statistics.SetCounter(100.0, api.PERF_DW_COUNT, "192.168.1.100")
		api.Statistics.SetCounter(100.0, api.PERF_UP_COUNT, "192.168.1.100")

		<-time.After(1 * time.Second)
		<-time.After(100 * time.Millisecond)

		upCount := api.Statistics.GetCounter(api.PERF_UP_SPEED, "192.168.1.100")
		dwCount := api.Statistics.GetCounter(api.PERF_DW_SPEED, "192.168.1.100")

		assert.NotEqual(s.T(), -1.0, upCount)
		assert.NotEqual(s.T(), -1.0, dwCount)
	}

	<-time.After(3 * time.Second)

	cancel()

	wg.Wait()

	upTotal := api.Statistics.GetCounter(api.PERF_UP_TOTAL, "192.168.1.100")
	dwTotal := api.Statistics.GetCounter(api.PERF_DW_TOTAL, "192.168.1.100")

	assert.Equal(s.T(), 300.0, upTotal)
	assert.Equal(s.T(), 300.0, dwTotal)

}

func (s *ServerSuite) TestListenQuicConn_Panic() {
	listenQuicConn(&testSession{})
}

func (s *ServerSuite) TestHandleQuicStream_Panic() {
	handleQuicStream(&testStream{})
}

func (s *ServerSuite) runServerTest() {
	ctx, cancel := newContext()

	var finished = false
	var wg = &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		RunServer(ctx, cancel)
		finished = true
	}()

	<-time.After(1 * time.Second)
	cancel()
	<-time.After(1 * time.Second)

	wg.Wait()

	assert.True(s.T(), finished)
}

func (s *ServerSuite) TestRunServer() {
	s.runServerTest()
}

func (s *ServerSuite) TestRunServer_BadConfig() {
	configuration.QPepConfig.Server.LocalListeningAddress = "ABCD"

	s.runServerTest()
}

func (s *ServerSuite) TestRunServer_APIConnection_BadHeader() {
	ctx, cancel := newContext()

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		RunServer(ctx, cancel)
	}()

	addr, _ := gateway.GetDefaultLanListeningAddress("127.0.0.1", "")

	conn, err := openQuicSession_test(addr, 9090)
	assert.Nil(s.T(), err)

	stream, err := conn.OpenStream(ctx)
	assert.Nil(s.T(), err)

	stream.Write([]byte{0, 0})

	receiveData := make([]byte, 1024)
	recv, _ := stream.Read(receiveData)

	assert.Equal(s.T(), 0, recv)

	stream.AbortWrite(0)
	stream.AbortRead(0)
	stream.Close()

	cancel()

	wg.Wait()
}

func (s *ServerSuite) TestRunServer_APIConnection_BadDestination() {
	ctx, cancel := newContext()

	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		RunServer(ctx, cancel)
	}()
	go func() {
		defer wg.Done()
		api.RunServer(ctx, cancel, false)
	}()

	addr, _ := gateway.GetDefaultLanListeningAddress("127.0.0.1", "")

	conn, err := openQuicSession_test(addr, 9090)
	assert.Nil(s.T(), err)

	stream, err := conn.OpenStream(ctx)
	assert.Nil(s.T(), err)

	sessionHeader := protocol.QPepHeader{
		SourceAddr: &net.TCPAddr{
			IP: net.ParseIP(addr),
		},
		DestAddr: &net.TCPAddr{
			IP:   net.ParseIP("0.0.0.0"),
			Port: 8080,
		},
	}

	stream.Write(sessionHeader.ToBytes())

	receiveData := make([]byte, 1024)
	recv, _ := stream.Read(receiveData)

	assert.Equal(s.T(), 0, recv)

	stream.AbortWrite(0)
	stream.AbortRead(0)
	stream.Close()

	cancel()

	wg.Wait()
}

func (s *ServerSuite) TestRunServer_APIConnection_LimitZeroSrc() {
	// incoming speed limit
	addr, _ := gateway.GetDefaultLanListeningAddress("127.0.0.1", "")

	clientsMap := map[string]string{
		addr + "/32": "0",
	}
	destMap := map[string]string{
		addr + "/32": "100K",
		"google.com": "0",
	}

	configuration.QPepConfig.Limits = &configuration.LimitsDefinition{
		Incoming: clientsMap,
		Outgoing: destMap,
	}

	// incoming speed limits
	configuration.LoadAddressSpeedLimitMap(clientsMap, true)

	// outgoing speed limit
	configuration.LoadAddressSpeedLimitMap(destMap, false)

	// launch request servers
	ctx, cancel := newContext()

	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		RunServer(ctx, cancel)
	}()
	go func() {
		defer wg.Done()
		api.RunServer(ctx, cancel, false)
	}()

	// open connection and send
	conn, err := openQuicSession_test(addr, 9090)
	assert.Nil(s.T(), err)

	stream, err := conn.OpenStream(ctx)
	assert.Nil(s.T(), err)

	sessionHeader := protocol.QPepHeader{
		SourceAddr: &net.TCPAddr{
			IP: net.ParseIP(addr),
		},
		DestAddr: &net.TCPAddr{
			IP:   net.ParseIP(addr),
			Port: 9443,
		},
	}

	stream.Write(sessionHeader.ToBytes())

	sendData := []byte("GET /api/v1/server/echo HTTP/1.1\r\nHost: :9443\r\nAccept: application/json\r\nAccept-Encoding: gzip\r\nConnection: close\r\nUser-Agent: windows\r\n\r\n\n")
	_, _ = stream.Write(sendData)

	receiveData := make([]byte, 1024)
	recv, _ := stream.Read(receiveData)

	matchStr := strings.ReplaceAll(string(receiveData[:recv]), "\r", "")

	assert.Len(s.T(), matchStr, 0)

	stream.AbortWrite(0)
	stream.AbortRead(0)
	stream.Close()

	cancel()

	wg.Wait()
}

func (s *ServerSuite) TestRunServer_APIConnection_LimitZeroDst() {
	// incoming speed limit
	addr, _ := gateway.GetDefaultLanListeningAddress("127.0.0.1", "")

	clientsMap := map[string]string{
		addr + "/32": "100K",
	}
	destMap := map[string]string{
		addr + "/32": "0",
		"google.com": "0",
	}

	configuration.QPepConfig.Limits = &configuration.LimitsDefinition{
		Incoming: clientsMap,
		Outgoing: destMap,
	}

	// incoming speed limits
	configuration.LoadAddressSpeedLimitMap(clientsMap, true)

	// outgoing speed limit
	configuration.LoadAddressSpeedLimitMap(destMap, false)

	// launch request servers
	ctx, cancel := newContext()

	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		RunServer(ctx, cancel)
	}()
	go func() {
		defer wg.Done()
		api.RunServer(ctx, cancel, false)
	}()

	// open connection and send
	conn, err := openQuicSession_test(addr, 9090)
	assert.Nil(s.T(), err)

	stream, err := conn.OpenStream(ctx)
	assert.Nil(s.T(), err)

	sessionHeader := protocol.QPepHeader{
		SourceAddr: &net.TCPAddr{
			IP: net.ParseIP(addr),
		},
		DestAddr: &net.TCPAddr{
			IP:   net.ParseIP(addr),
			Port: 9443,
		},
	}

	stream.Write(sessionHeader.ToBytes())

	sendData := []byte("GET /api/v1/server/echo HTTP/1.1\r\nHost: :9443\r\nAccept: application/json\r\nAccept-Encoding: gzip\r\nConnection: close\r\nUser-Agent: windows\r\n\r\n\n")
	_, _ = stream.Write(sendData)

	receiveData := make([]byte, 1024)
	recv, _ := stream.Read(receiveData)

	matchStr := strings.ReplaceAll(string(receiveData[:recv]), "\r", "")

	assert.Len(s.T(), matchStr, 0)

	stream.AbortWrite(0)
	stream.AbortRead(0)
	stream.Close()

	cancel()

	wg.Wait()
}

func (s *ServerSuite) TestRunServer_APIConnection_LimitSrc() {
	// incoming speed limit
	addr, _ := gateway.GetDefaultLanListeningAddress("127.0.0.1", "")

	s.T().Logf("address: %v", addr)

	clientsMap := map[string]string{
		addr + "/32": "300K",
	}
	destMap := map[string]string{
		"google.com": "0",
	}

	configuration.QPepConfig.Limits = &configuration.LimitsDefinition{
		Incoming: clientsMap,
		Outgoing: destMap,
	}

	// incoming speed limits
	configuration.LoadAddressSpeedLimitMap(clientsMap, true)

	// outgoing speed limit
	configuration.LoadAddressSpeedLimitMap(destMap, false)

	// launch request servers
	ctx, cancel := newContext()

	var apisrv *http.Server = nil

	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		RunServer(ctx, cancel)
	}()
	go func() {
		defer wg.Done()

		rtr := httprouter.New()
		rtr.RedirectTrailingSlash = true
		rtr.RedirectFixedPath = true
		rtr.Handle(http.MethodPost, "/testapi", func(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
			data, _ := ioutil.ReadAll(req.Body)

			logger.Info("Read: %d", len(data))

			w.WriteHeader(http.StatusOK)
			w.Header().Add("Content-Type", "text/html")
			w.Write([]byte("OK\r\n"))
		})
		corsRouterHandler := cors.Default().Handler(rtr)

		apisrv = &http.Server{
			Addr:    fmt.Sprintf("%s:%d", addr, 9443),
			Handler: corsRouterHandler,
			BaseContext: func(l net.Listener) context.Context {
				return ctx
			},
		}

		if err := apisrv.ListenAndServe(); err != nil {
			testlog.Info().Msgf("Error running API server: %v", err)
		}
		apisrv = nil
	}()

	// open connection and send
	conn, err := openQuicSession_test(addr, 9090)
	assert.Nil(s.T(), err)

	stream, err := conn.OpenStream(ctx)
	assert.Nil(s.T(), err)

	sessionHeader := protocol.QPepHeader{
		SourceAddr: &net.TCPAddr{
			IP: net.ParseIP(addr),
		},
		DestAddr: &net.TCPAddr{
			IP:   net.ParseIP(addr),
			Port: 9443,
		},
	}

	var startSend = time.Now()
	stream.Write(sessionHeader.ToBytes())

	sendData := []byte("POST /testapi HTTP/1.1\r\nHost: :9443\r\nAccept: application/json\r\nAccept-Encoding: gzip\r\nConnection: close\r\nContent-Length: 1048576\r\nUser-Agent: windows\r\n\r\n" +
		strings.Repeat("N", 1024*1024))

	written, err := stream.Write(sendData)
	assert.Equal(s.T(), written, len(sendData))
	assert.Nil(s.T(), err)

	stream.Sync()

	receiveData := make([]byte, 1024)
	stream.SetReadDeadline(time.Now().Add(3 * time.Second))
	recv, _ := stream.Read(receiveData)
	var sendEnd = time.Now()

	assert.Contains(s.T(), string(receiveData[:recv]), `HTTP/1.1 200 OK`)

	stream.AbortWrite(0)
	stream.AbortRead(0)
	stream.Close()
	cancel()

	apisrv.Close()

	wg.Wait()

	// very bland check for 300k/s upload speed
	assert.True(s.T(), sendEnd.Sub(startSend) >= 3*time.Second)
}

func (s *ServerSuite) TestRunServer_APIConnection_LimitDst() {
	// incoming speed limit
	addr, _ := gateway.GetDefaultLanListeningAddress("127.0.0.1", "")

	clientsMap := map[string]string{}
	destMap := map[string]string{
		addr + "/32": "300K",
		"google.com": "0",
	}

	configuration.QPepConfig.Limits = &configuration.LimitsDefinition{
		Incoming: clientsMap,
		Outgoing: destMap,
	}

	// incoming speed limits
	configuration.LoadAddressSpeedLimitMap(clientsMap, true)

	// outgoing speed limit
	configuration.LoadAddressSpeedLimitMap(destMap, false)

	// launch request servers
	ctx, cancel := newContext()

	var apisrv *http.Server = nil
	var expectSent = 0

	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		RunServer(ctx, cancel)
	}()
	go func() {
		defer wg.Done()

		rtr := httprouter.New()
		rtr.RedirectTrailingSlash = true
		rtr.RedirectFixedPath = true
		rtr.Handle(http.MethodGet, "/testapi", func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
			w.WriteHeader(http.StatusOK)
			w.Header().Add("Content-Type", "text/html")
			for i := 0; i < 1024; i++ {
				sent, _ := w.Write([]byte(strings.Repeat("X", 1024) + "\n"))
				expectSent += sent
			}
		})
		corsRouterHandler := cors.Default().Handler(rtr)

		apisrv = &http.Server{
			Addr:    fmt.Sprintf("%s:%d", addr, 9443),
			Handler: corsRouterHandler,
			BaseContext: func(l net.Listener) context.Context {
				return ctx
			},
		}

		if err := apisrv.ListenAndServe(); err != nil {
			testlog.Info().Msgf("Error running API server: %v", err)
		}
		apisrv = nil
	}()

	// open connection and send
	conn, err := openQuicSession_test(addr, 9090)
	assert.Nil(s.T(), err)

	stream, err := conn.OpenStream(ctx)
	assert.Nil(s.T(), err)

	sessionHeader := protocol.QPepHeader{
		SourceAddr: &net.TCPAddr{
			IP: net.ParseIP(addr),
		},
		DestAddr: &net.TCPAddr{
			IP:   net.ParseIP(addr),
			Port: 9443,
		},
	}

	var startSend = time.Now()
	stream.Write(sessionHeader.ToBytes())

	sendData := []byte("GET /testapi HTTP/1.1\r\nHost: :9443\r\nAccept: */*\r\nAccept-Encoding: gzip\r\nConnection: close\r\nUser-Agent: windows\r\n\r\n\n")

	stream.Write(sendData)

	var sendEnd time.Time
	total := 0
	scn := bufio.NewScanner(stream)
	for scn.Scan() {
		total += len(scn.Bytes())
		testlog.Info().Msgf("total: %d / %d / %d - %v", total, expectSent, (1024+1)*1024, time.Now().Sub(startSend))

		if total > expectSent {
			sendEnd = time.Now()

			stream.AbortRead(0)
			stream.AbortWrite(0)
			stream.Close()
			break
		}
	}

	cancel()

	_ = apisrv.Close()

	wg.Wait()

	assert.True(s.T(), total > expectSent)

	// very bland check for 300k/s upload speed
	assert.True(s.T(), sendEnd.Sub(startSend) > 3*time.Second)
	assert.True(s.T(), sendEnd.Sub(startSend) < 6*time.Second)
}

func (s *ServerSuite) TestRunServer_DownloadConnection() {
	// incoming speed limit
	addr, _ := gateway.GetDefaultLanListeningAddress("127.0.0.1", "")

	clientsMap := map[string]string{}
	destMap := map[string]string{}

	configuration.QPepConfig.Limits = &configuration.LimitsDefinition{
		Incoming: clientsMap,
		Outgoing: destMap,
	}

	// incoming speed limits
	configuration.LoadAddressSpeedLimitMap(clientsMap, true)

	// outgoing speed limit
	configuration.LoadAddressSpeedLimitMap(destMap, false)

	// launch request servers
	ctx, cancel := newContext()

	var apisrv *http.Server = nil
	var expectSent = 1024 * 1024 * 10

	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		RunServer(ctx, cancel)
	}()
	go func() {
		defer wg.Done()
		rtr := httprouter.New()
		rtr.RedirectTrailingSlash = true
		rtr.RedirectFixedPath = true
		rtr.Handle(http.MethodGet, "/testapi", func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
			w.WriteHeader(http.StatusOK)
			w.Header().Add("Content-Type", "text/html")

			data := strings.Repeat("X", 1024*1024*5)
			sent, _ := w.Write([]byte(data))
			<-time.After(1 * time.Second)
			assert.Equal(s.T(), 1024*1024*5, sent)
			data = strings.Repeat("X", 1024*1024*5)
			sent, _ = w.Write([]byte(data))
		})
		corsRouterHandler := cors.Default().Handler(rtr)

		apisrv = &http.Server{
			Addr:    fmt.Sprintf("%s:%d", addr, 9443),
			Handler: corsRouterHandler,
			BaseContext: func(l net.Listener) context.Context {
				return ctx
			},
		}

		if err := apisrv.ListenAndServe(); err != nil {
			testlog.Info().Msgf("Error running API server: %v", err)
		}
		apisrv = nil
	}()

	// open connection and send
	conn, err := openQuicSession_test(addr, 9090)
	assert.Nil(s.T(), err)

	stream, err := conn.OpenStream(ctx)
	assert.Nil(s.T(), err)

	sessionHeader := protocol.QPepHeader{
		SourceAddr: &net.TCPAddr{
			IP: net.ParseIP(addr),
		},
		DestAddr: &net.TCPAddr{
			IP:   net.ParseIP(addr),
			Port: 9443,
		},
	}

	stream.Write(sessionHeader.ToBytes())

	sendData := []byte("GET /testapi HTTP/1.1\r\nHost: :9443\r\nAccept: */*\r\nAccept-Encoding: gzip\r\nConnection: close\r\nUser-Agent: windows\r\n\r\n\n")

	stream.Write(sendData)

	out, _ := io.ReadAll(stream)

	cancel()

	_ = apisrv.Close()

	wg.Wait()

	assert.True(s.T(), len(out) >= expectSent)
}

func (s *ServerSuite) TestRunServer_DownloadConnection_InactivityTimeout() {
	// incoming speed limit
	addr, _ := gateway.GetDefaultLanListeningAddress("127.0.0.1", "")

	clientsMap := map[string]string{}
	destMap := map[string]string{}

	configuration.QPepConfig.Limits = &configuration.LimitsDefinition{
		Incoming: clientsMap,
		Outgoing: destMap,
	}

	// incoming speed limits
	configuration.LoadAddressSpeedLimitMap(clientsMap, true)

	// outgoing speed limit
	configuration.LoadAddressSpeedLimitMap(destMap, false)

	// launch request servers
	ctx, cancel := newContext()

	var apisrv *http.Server = nil
	var expectSent = 1024 * 1024 * 10

	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		RunServer(ctx, cancel)
	}()
	go func() {
		defer wg.Done()
		rtr := httprouter.New()
		rtr.RedirectTrailingSlash = true
		rtr.RedirectFixedPath = true
		rtr.Handle(http.MethodGet, "/testapi", func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
			w.WriteHeader(http.StatusOK)
			w.Header().Add("Content-Type", "text/html")
			data := strings.Repeat("X", 1024*1024*5)
			sent, _ := w.Write([]byte(data))
			<-time.After(20 * time.Second)
			assert.Equal(s.T(), 1024*1024*5, sent)
			data = strings.Repeat("X", 1024*1024*5)
			sent, _ = w.Write([]byte(data))
		})
		corsRouterHandler := cors.Default().Handler(rtr)

		apisrv = &http.Server{
			Addr:    fmt.Sprintf("%s:%d", addr, 9443),
			Handler: corsRouterHandler,
			BaseContext: func(l net.Listener) context.Context {
				return ctx
			},
		}

		if err := apisrv.ListenAndServe(); err != nil {
			testlog.Info().Msgf("Error running API server: %v", err)
		}
		apisrv = nil
	}()

	// open connection and send
	conn, err := openQuicSession_test(addr, 9090)
	assert.Nil(s.T(), err)

	stream, err := conn.OpenStream(ctx)
	assert.Nil(s.T(), err)

	sessionHeader := protocol.QPepHeader{
		SourceAddr: &net.TCPAddr{
			IP: net.ParseIP(addr),
		},
		DestAddr: &net.TCPAddr{
			IP:   net.ParseIP(addr),
			Port: 9443,
		},
	}

	stream.Write(sessionHeader.ToBytes())

	sendData := []byte("GET /testapi HTTP/1.1\r\nHost: :9443\r\nAccept: */*\r\nAccept-Encoding: gzip\r\nUser-Agent: windows\r\n\r\n\n")

	stream.Write(sendData)

	out, err := ioutil.ReadAll(stream)
	assert.NotNil(s.T(), err)

	cancel()

	_ = apisrv.Close()

	wg.Wait()

	testlog.Info().Msgf("size %d >= %d: %v\n", len(out), expectSent, len(out) >= expectSent)

	assert.True(s.T(), len(out) < expectSent)
}

// --- utilities --- //
func newContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	var lastError error
	ctx = context.WithValue(ctx, "lastError", &lastError)

	return ctx, cancel
}

func openQuicSession_test(address string, port int) (backend.QuicBackendConnection, error) {
	var ok bool
	quicProvider, ok = backend.Get(configuration.QPepConfig.Protocol.Backend)
	if !ok {
		panic(stderr.ErrInvalidBackendSelected)
	}

	conn, err := quicProvider.Dial(context.Background(), address, port, "cert.pem",
		"reno", "basic", false)

	if err != nil {
		logger.Error("Unrecoverable error while listening for Protocol connections: %s\n", err)
		return nil, err
	}

	return conn, nil
}

type testBackend struct{}

func (t testBackend) Dial(ctx context.Context, remoteAddress string, port int, clientCertPath string,
	ccAlgorithm string, ccSlowstartAlgo string, traceOn bool) (backend.QuicBackendConnection, error) {
	panic("test-error")
}

func (t testBackend) Listen(ctx context.Context, address string, port int, serverCertPath string, serverKeyPath string,
	ccAlgorithm string, ccSlowstartAlgo string, traceOn bool) (backend.QuicBackendConnection, error) {
	panic("test-error")
}

func (t testBackend) Close() error {
	panic("test-error")
}

type testListener struct{}

func (t *testListener) LocalAddr() net.Addr {
	panic("test-error")
}

func (t *testListener) RemoteAddr() net.Addr {
	panic("test-error")
}

func (t *testListener) OpenStream(ctx context.Context) (backend.QuicBackendStream, error) {
	panic("test-error")
}

func (t *testListener) AcceptStream(ctx context.Context) (backend.QuicBackendStream, error) {
	panic("test-error")
}

func (t *testListener) AcceptConnection(ctx context.Context) (backend.QuicBackendConnection, error) {
	panic("test-error")
}

func (t *testListener) Close(code int, message string) error {
	return nil
}

func (t *testListener) IsClosed() bool {
	panic("test-error")
}

func (t *testListener) Addr() net.Addr {
	return nil
}

type testSession struct{}

func (t testSession) OpenStream(ctx context.Context) (backend.QuicBackendStream, error) {
	panic("test-error")
}

func (t testSession) AcceptStream(ctx context.Context) (backend.QuicBackendStream, error) {
	panic("test-error")
}

func (t testSession) AcceptConnection(ctx context.Context) (backend.QuicBackendConnection, error) {
	panic("test-error")
}

func (t testSession) Close(code int, message string) error {
	panic("test-error")
}

func (t testSession) IsClosed() bool {
	panic("test-error")
}

func (t testSession) LocalAddr() net.Addr {
	panic("test-error")
}

func (t testSession) RemoteAddr() net.Addr {
	panic("test-error")
}

var _ backend.QuicBackendConnection = &testSession{}

type testStream struct{}

func (t testStream) ID() uint64 {
	panic("test-error")
}

func (t testStream) Sync() bool {
	panic("test-error")
}

func (t testStream) AbortRead(code uint64) {
	panic("test-error")
}

func (t testStream) AbortWrite(code uint64) {
	panic("test-error")
}

func (t testStream) IsClosed() bool {
	panic("test-error")
}

func (t testStream) Read(p []byte) (n int, err error) {
	panic("test-error")
}

func (t testStream) SetReadDeadline(tm time.Time) error {
	panic("test-error")
}

func (t testStream) Write(p []byte) (n int, err error) {
	panic("test-error")
}

func (t testStream) Close() error {
	panic("test-error")
}

func (t testStream) SetWriteDeadline(tm time.Time) error {
	panic("test-error")
}

var _ backend.QuicBackendStream = &testStream{}
