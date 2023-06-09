package server

import (
	"bou.ke/monkey"
	"bufio"
	"context"
	"crypto/tls"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/Project-Faster/quic-go"
	"github.com/julienschmidt/httprouter"
	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/shared"
	"github.com/rs/cors"
	log "github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

var testlog log.Logger

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
	shared.QPepConfig.ListenHost = "127.0.0.1"
	shared.QPepConfig.ListenPort = 9090
	shared.QPepConfig.GatewayAPIPort = 9443
}

func (s *ServerSuite) AfterTest(_, testName string) {
	monkey.UnpatchAll()
	api.Statistics.Reset()
}

func (s *ServerSuite) TestValidateConfiguration() {
	assert.NotPanics(s.T(), func() {
		validateConfiguration()
	})
}

func (s *ServerSuite) TestValidateConfiguration_BadListenAddress() {
	shared.QPepConfig.ListenHost = "ABCD"
	shared.QPepConfig.ListenPort = 9090
	shared.QPepConfig.GatewayAPIPort = 9443

	assert.PanicsWithValue(s.T(), shared.ErrConfigurationValidationFailed, func() {
		validateConfiguration()
	})
}

func (s *ServerSuite) TestValidateConfiguration_BadListenPort() {
	shared.QPepConfig.ListenHost = "127.0.0.1"
	shared.QPepConfig.ListenPort = 0
	shared.QPepConfig.GatewayAPIPort = 9443

	assert.PanicsWithValue(s.T(), shared.ErrConfigurationValidationFailed, func() {
		validateConfiguration()
	})
}

func (s *ServerSuite) TestValidateConfiguration_BadAPIPort() {
	shared.QPepConfig.ListenHost = "127.0.0.1"
	shared.QPepConfig.ListenPort = 9090
	shared.QPepConfig.GatewayAPIPort = 99999

	assert.PanicsWithValue(s.T(), shared.ErrConfigurationValidationFailed, func() {
		validateConfiguration()
	})
}

func (s *ServerSuite) TestPerformanceWatcher() {
	ctx, cancel := context.WithCancel(context.Background())

	api.Statistics.SetMappedAddress("127.0.0.1", "192.168.1.100")

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		performanceWatcher(ctx)
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

func (s *ServerSuite) TestPerformanceWatcher_Panic() {
	var prevStats = api.Statistics
	api.Statistics = nil
	defer func() {
		api.Statistics = prevStats
	}()

	assert.NotPanics(s.T(), func() {
		performanceWatcher(context.Background())
	})
}

func (s *ServerSuite) TestGenerateTLSConfig() {
	config := generateTLSConfig()
	assert.NotNil(s.T(), config)

	assert.NotNil(s.T(), config.Certificates)
	assert.NotNil(s.T(), config.Certificates[0])

	assert.Equal(s.T(), "qpep", config.NextProtos[0])

	data, err := ioutil.ReadFile("server_cert.pem")
	assert.Nil(s.T(), err)

	cert, _ := pem.Decode(data)
	assert.NotNil(s.T(), cert)

	_, err = os.Stat("server_key.pem")
	assert.Nil(s.T(), err)
}

func (s *ServerSuite) TestListenQuicSession_Panic() {
	quicListener = &testListener{}
	listenQuicSession()
}

func (s *ServerSuite) TestListenQuicConn_Panic() {
	listenQuicConn(&testSession{})
}

func (s *ServerSuite) TestHandleQuicStream_Panic() {
	handleQuicStream(&testStream{})
}

func (s *ServerSuite) TestRunServer() {
	ctx, cancel := context.WithCancel(context.Background())

	var finished = false
	go func() {
		RunServer(ctx, cancel)
		finished = true
	}()

	<-time.After(1 * time.Second)
	cancel()
	<-time.After(1 * time.Second)

	assert.True(s.T(), finished)
}

func (s *ServerSuite) TestRunServer_BadConfig() {
	ctx, cancel := context.WithCancel(context.Background())

	shared.QPepConfig.ListenHost = "ABCD"

	RunServer(ctx, cancel)
}

func (s *ServerSuite) TestRunServer_BadListener() {
	ctx, cancel := context.WithCancel(context.Background())

	monkey.Patch(quic.ListenAddr, func(string, *tls.Config, *quic.Config) (quic.Listener, error) {
		return nil, errors.New("test-error")
	})

	RunServer(ctx, cancel)
}

func (s *ServerSuite) TestRunServer_APIConnection() {
	ctx, cancel := context.WithCancel(context.Background())

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

	addr, _ := shared.GetDefaultLanListeningAddress("127.0.0.1", "")

	conn, err := openQuicSession_test(addr, 9090)
	assert.Nil(s.T(), err)

	stream, err := conn.OpenStreamSync(ctx)
	assert.Nil(s.T(), err)

	sessionHeader := shared.QPepHeader{
		SourceAddr: &net.TCPAddr{
			IP: net.ParseIP(addr),
		},
		DestAddr: &net.TCPAddr{
			IP:   net.ParseIP(addr),
			Port: 9443,
		},
	}

	stream.Write(sessionHeader.ToBytes())

	sendData := []byte("GET /api/v1/server/echo HTTP/1.1\r\nHost: :9443\r\nAccept: application/json\r\nAccept-Encoding: gzip\r\nUser-Agent: windows\r\n\r\n\n")
	_, _ = stream.Write(sendData)

	receiveData := make([]byte, 1024)
	recv, _ := stream.Read(receiveData)

	expectedRecv := `HTTP\/1\.1 200 OK
Content-Type: application\/json
Vary: Origin
Date: .+ GMT
Content-Length: \d+

{"address":"[^"]+","port":0,"serverversion":"[^"]+","total_connections":\d+}`

	matchStr := strings.ReplaceAll(string(receiveData[:recv]), "\r", "")

	re := regexp.MustCompile(expectedRecv)
	assert.True(s.T(), re.MatchString(matchStr))

	stream.CancelWrite(0)
	stream.CancelRead(0)
	stream.Close()

	cancel()

	wg.Wait()
}

func (s *ServerSuite) TestRunServer_APIConnection_BadHeader() {
	ctx, cancel := context.WithCancel(context.Background())

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		RunServer(ctx, cancel)
	}()

	addr, _ := shared.GetDefaultLanListeningAddress("127.0.0.1", "")

	conn, err := openQuicSession_test(addr, 9090)
	assert.Nil(s.T(), err)

	stream, err := conn.OpenStreamSync(ctx)
	assert.Nil(s.T(), err)

	stream.Write([]byte{0, 0})

	receiveData := make([]byte, 1024)
	recv, _ := stream.Read(receiveData)

	assert.Equal(s.T(), 0, recv)

	stream.CancelWrite(0)
	stream.CancelRead(0)
	stream.Close()

	cancel()

	wg.Wait()
}

func (s *ServerSuite) TestRunServer_APIConnection_BadDestination() {
	ctx, cancel := context.WithCancel(context.Background())

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

	addr, _ := shared.GetDefaultLanListeningAddress("127.0.0.1", "")

	conn, err := openQuicSession_test(addr, 9090)
	assert.Nil(s.T(), err)

	stream, err := conn.OpenStreamSync(ctx)
	assert.Nil(s.T(), err)

	sessionHeader := shared.QPepHeader{
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

	stream.CancelWrite(0)
	stream.CancelRead(0)
	stream.Close()

	cancel()

	wg.Wait()
}

func (s *ServerSuite) TestRunServer_APIConnection_LimitZeroSrc() {
	// incoming speed limit
	addr, _ := shared.GetDefaultLanListeningAddress("127.0.0.1", "")

	clientsMap := map[string]string{
		addr + "/32": "0",
	}
	destMap := map[string]string{
		addr + "/32": "100K",
		"google.com": "0",
	}

	shared.QPepConfig.Limits = shared.LimitsDefinition{
		Clients:      clientsMap,
		Destinations: destMap,
	}

	// incoming speed limits
	shared.LoadAddressSpeedLimitMap(clientsMap, true)

	// outgoing speed limit
	shared.LoadAddressSpeedLimitMap(destMap, false)

	// launch request servers
	ctx, cancel := context.WithCancel(context.Background())

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

	stream, err := conn.OpenStreamSync(ctx)
	assert.Nil(s.T(), err)

	sessionHeader := shared.QPepHeader{
		SourceAddr: &net.TCPAddr{
			IP: net.ParseIP(addr),
		},
		DestAddr: &net.TCPAddr{
			IP:   net.ParseIP(addr),
			Port: 9443,
		},
	}

	stream.Write(sessionHeader.ToBytes())

	sendData := []byte("GET /api/v1/server/echo HTTP/1.1\r\nHost: :9443\r\nAccept: application/json\r\nAccept-Encoding: gzip\r\nUser-Agent: windows\r\n\r\n\n")
	_, _ = stream.Write(sendData)

	receiveData := make([]byte, 1024)
	recv, _ := stream.Read(receiveData)

	matchStr := strings.ReplaceAll(string(receiveData[:recv]), "\r", "")

	assert.Len(s.T(), matchStr, 0)

	stream.CancelWrite(0)
	stream.CancelRead(0)
	stream.Close()

	cancel()

	wg.Wait()
}

func (s *ServerSuite) TestRunServer_APIConnection_LimitZeroDst() {
	// incoming speed limit
	addr, _ := shared.GetDefaultLanListeningAddress("127.0.0.1", "")

	clientsMap := map[string]string{
		addr + "/32": "100K",
	}
	destMap := map[string]string{
		addr + "/32": "0",
		"google.com": "0",
	}

	shared.QPepConfig.Limits = shared.LimitsDefinition{
		Clients:      clientsMap,
		Destinations: destMap,
	}

	// incoming speed limits
	shared.LoadAddressSpeedLimitMap(clientsMap, true)

	// outgoing speed limit
	shared.LoadAddressSpeedLimitMap(destMap, false)

	// launch request servers
	ctx, cancel := context.WithCancel(context.Background())

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

	stream, err := conn.OpenStreamSync(ctx)
	assert.Nil(s.T(), err)

	sessionHeader := shared.QPepHeader{
		SourceAddr: &net.TCPAddr{
			IP: net.ParseIP(addr),
		},
		DestAddr: &net.TCPAddr{
			IP:   net.ParseIP(addr),
			Port: 9443,
		},
	}

	stream.Write(sessionHeader.ToBytes())

	sendData := []byte("GET /api/v1/server/echo HTTP/1.1\r\nHost: :9443\r\nAccept: application/json\r\nAccept-Encoding: gzip\r\nUser-Agent: windows\r\n\r\n\n")
	_, _ = stream.Write(sendData)

	receiveData := make([]byte, 1024)
	recv, _ := stream.Read(receiveData)

	matchStr := strings.ReplaceAll(string(receiveData[:recv]), "\r", "")

	assert.Len(s.T(), matchStr, 0)

	stream.CancelWrite(0)
	stream.CancelRead(0)
	stream.Close()

	cancel()

	wg.Wait()
}

func (s *ServerSuite) TestRunServer_APIConnection_LimitSrc() {
	// incoming speed limit
	addr, _ := shared.GetDefaultLanListeningAddress("127.0.0.1", "")

	clientsMap := map[string]string{
		addr + "/32": "300K",
	}
	destMap := map[string]string{
		"google.com": "0",
	}

	shared.QPepConfig.Limits = shared.LimitsDefinition{
		Clients:      clientsMap,
		Destinations: destMap,
	}

	// incoming speed limits
	shared.LoadAddressSpeedLimitMap(clientsMap, true)

	// outgoing speed limit
	shared.LoadAddressSpeedLimitMap(destMap, false)

	// launch request servers
	ctx, cancel := context.WithCancel(context.Background())

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
		rtr.Handle(http.MethodPost, "/testapi", func(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
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

	stream, err := conn.OpenStreamSync(ctx)
	assert.Nil(s.T(), err)

	sessionHeader := shared.QPepHeader{
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

	sendData := []byte("POST /testapi HTTP/1.1\r\nHost: :9443\r\nAccept: application/json\r\nAccept-Encoding: gzip\r\nUser-Agent: windows\r\n" +
		strings.Repeat("N", 1024*1024) +
		"\r\n\n")

	written, err := stream.Write(sendData)
	assert.True(s.T(), written >= 0)
	assert.Nil(s.T(), err)

	receiveData := make([]byte, 1024)
	recv, _ := stream.Read(receiveData)
	var sendEnd = time.Now()

	matchStr := strings.ReplaceAll(string(receiveData[:recv]), "\r", "")

	assert.Equal(s.T(), `HTTP/1.1 400 Bad Request
Content-Type: text/plain; charset=utf-8
Connection: close

400 Bad Request`, matchStr)

	stream.CancelWrite(0)
	stream.CancelRead(0)
	stream.Close()

	cancel()

	apisrv.Close()

	wg.Wait()

	// very bland check for 300k/s upload speed
	assert.True(s.T(), sendEnd.Sub(startSend) >= 3*time.Second)
	assert.True(s.T(), sendEnd.Sub(startSend) < 6*time.Second)
}

func (s *ServerSuite) TestRunServer_APIConnection_LimitDst() {
	// incoming speed limit
	addr, _ := shared.GetDefaultLanListeningAddress("127.0.0.1", "")

	clientsMap := map[string]string{}
	destMap := map[string]string{
		addr + "/32": "300K",
		"google.com": "0",
	}

	shared.QPepConfig.Limits = shared.LimitsDefinition{
		Clients:      clientsMap,
		Destinations: destMap,
	}

	// incoming speed limits
	shared.LoadAddressSpeedLimitMap(clientsMap, true)

	// outgoing speed limit
	shared.LoadAddressSpeedLimitMap(destMap, false)

	// launch request servers
	ctx, cancel := context.WithCancel(context.Background())

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

	stream, err := conn.OpenStreamSync(ctx)
	assert.Nil(s.T(), err)

	sessionHeader := shared.QPepHeader{
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

	sendData := []byte("GET /testapi HTTP/1.1\r\nHost: :9443\r\nAccept: */*\r\nAccept-Encoding: gzip\r\nUser-Agent: windows\r\n\r\n\n")

	stream.Write(sendData)

	var sendEnd time.Time
	total := 0
	scn := bufio.NewScanner(stream)
	for scn.Scan() {
		total += len(scn.Bytes())
		testlog.Info().Msgf("total: %d / %d / %d - %v", total, expectSent, (1024+1)*1024, time.Now().Sub(startSend))

		if total > expectSent {
			sendEnd = time.Now()

			stream.CancelRead(0)
			stream.CancelWrite(0)
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
	addr, _ := shared.GetDefaultLanListeningAddress("127.0.0.1", "")

	clientsMap := map[string]string{}
	destMap := map[string]string{}

	shared.QPepConfig.Limits = shared.LimitsDefinition{
		Clients:      clientsMap,
		Destinations: destMap,
	}

	// incoming speed limits
	shared.LoadAddressSpeedLimitMap(clientsMap, true)

	// outgoing speed limit
	shared.LoadAddressSpeedLimitMap(destMap, false)

	// launch request servers
	ctx, cancel := context.WithCancel(context.Background())

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

	stream, err := conn.OpenStreamSync(ctx)
	assert.Nil(s.T(), err)

	sessionHeader := shared.QPepHeader{
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
	assert.Nil(s.T(), err)

	cancel()

	_ = apisrv.Close()

	wg.Wait()

	assert.True(s.T(), len(out) >= expectSent)
}

func (s *ServerSuite) TestRunServer_DownloadConnection_InactivityTimeout() {
	// incoming speed limit
	addr, _ := shared.GetDefaultLanListeningAddress("127.0.0.1", "")

	clientsMap := map[string]string{}
	destMap := map[string]string{}

	shared.QPepConfig.Limits = shared.LimitsDefinition{
		Clients:      clientsMap,
		Destinations: destMap,
	}

	// incoming speed limits
	shared.LoadAddressSpeedLimitMap(clientsMap, true)

	// outgoing speed limit
	shared.LoadAddressSpeedLimitMap(destMap, false)

	// launch request servers
	ctx, cancel := context.WithCancel(context.Background())

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
			<-time.After(4 * time.Second)
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

	stream, err := conn.OpenStreamSync(ctx)
	assert.Nil(s.T(), err)

	sessionHeader := shared.QPepHeader{
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
	assert.Nil(s.T(), err)

	cancel()

	_ = apisrv.Close()

	wg.Wait()

	testlog.Info().Msgf("size %d >= %d: %v\n", len(out), expectSent, len(out) >= expectSent)

	assert.True(s.T(), len(out) < expectSent)
}

// --- utilities --- //
func openQuicSession_test(address string, port int) (quic.Connection, error) {
	config := &quic.Config{DisablePathMTUDiscovery: true}
	tlsConf := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"qpep"}}
	gatewayPath := fmt.Sprintf("%s:%d", address, port) // "192.168.1.89:9090"

	testlog.Info().Msgf("Dialing QUIC Session: %s\n", gatewayPath)
	return quic.DialAddr(gatewayPath, tlsConf, config)
}

type testListener struct{}

func (t *testListener) Accept(ctx context.Context) (quic.Connection, error) {
	panic("test-error")
}
func (t *testListener) Addr() net.Addr {
	return nil
}
func (t *testListener) Close() error {
	return nil
}

type testSession struct{}

func (t testSession) AcceptStream(ctx context.Context) (quic.Stream, error) {
	panic("test-error")
}

func (t testSession) AcceptUniStream(ctx context.Context) (quic.ReceiveStream, error) {
	panic("test-error")
}

func (t testSession) OpenStream() (quic.Stream, error) {
	panic("test-error")
}

func (t testSession) OpenStreamSync(ctx context.Context) (quic.Stream, error) {
	panic("test-error")
}

func (t testSession) OpenUniStream() (quic.SendStream, error) {
	panic("test-error")
}

func (t testSession) OpenUniStreamSync(ctx context.Context) (quic.SendStream, error) {
	panic("test-error")
}

func (t testSession) LocalAddr() net.Addr {
	panic("test-error")
}

func (t testSession) RemoteAddr() net.Addr {
	panic("test-error")
}

func (t testSession) CloseWithError(code quic.ApplicationErrorCode, s string) error {
	panic("test-error")
}

func (t testSession) Context() context.Context {
	panic("test-error")
}

func (t testSession) ConnectionState() quic.ConnectionState {
	panic("test-error")
}

func (t testSession) SendMessage(bytes []byte) error {
	panic("test-error")
}

func (t testSession) ReceiveMessage() ([]byte, error) {
	panic("test-error")
}

var _ quic.Connection = &testSession{}

type testStream struct{}

func (t testStream) StreamID() quic.StreamID {
	panic("test-error")
}

func (t testStream) Read(p []byte) (n int, err error) {
	panic("test-error")
}

func (t testStream) CancelRead(code quic.StreamErrorCode) {
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

func (t testStream) CancelWrite(code quic.StreamErrorCode) {
	panic("test-error")
}

func (t testStream) Context() context.Context {
	panic("test-error")
}

func (t testStream) SetWriteDeadline(tm time.Time) error {
	panic("test-error")
}

func (t testStream) SetDeadline(tm time.Time) error {
	panic("test-error")
}

var _ quic.Stream = &testStream{}
