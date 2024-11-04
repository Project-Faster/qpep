//go:build !arm64

package server

import (
	"crypto/tls"
	stderr "errors"
	"github.com/Project-Faster/monkey"
	"github.com/Project-Faster/qpep/api"
	"github.com/Project-Faster/qpep/shared/configuration"
	"github.com/Project-Faster/qpep/shared/errors"
	"github.com/Project-Faster/qpep/shared/protocol"
	"github.com/Project-Faster/qpep/workers/gateway"
	"github.com/Project-Faster/quic-go"
	"github.com/stretchr/testify/assert"
	"net"
	"regexp"
	"strings"
	"sync"
)

func (s *ServerSuite) TestValidateConfiguration() {
	assert.NotPanics(s.T(), func() {
		validateConfiguration()
	})
}

func (s *ServerSuite) TestValidateConfiguration_BadListenAddress() {
	configuration.QPepConfig.Server.LocalListeningAddress = "ABCD"
	configuration.QPepConfig.Server.LocalListenPort = 9090
	configuration.QPepConfig.General.APIPort = 9443

	assert.PanicsWithValue(s.T(), errors.ErrConfigurationValidationFailed, func() {
		validateConfiguration()
	})
}

func (s *ServerSuite) TestValidateConfiguration_BadListenPort() {
	configuration.QPepConfig.Server.LocalListeningAddress = "127.0.0.1"
	configuration.QPepConfig.Server.LocalListenPort = 0
	configuration.QPepConfig.General.APIPort = 9443

	assert.PanicsWithValue(s.T(), errors.ErrConfigurationValidationFailed, func() {
		validateConfiguration()
	})
}

func (s *ServerSuite) TestValidateConfiguration_BadAPIPort() {
	configuration.QPepConfig.Server.LocalListeningAddress = "127.0.0.1"
	configuration.QPepConfig.Server.LocalListenPort = 9090
	configuration.QPepConfig.General.APIPort = 99999

	assert.PanicsWithValue(s.T(), errors.ErrConfigurationValidationFailed, func() {
		validateConfiguration()
	})
}

func (s *ServerSuite) TestRunServer_BadListener() {
	monkey.Patch(quic.ListenAddr, func(string, *tls.Config, *quic.Config) (quic.Listener, error) {
		return nil, stderr.New("test-error")
	})

	s.runServerTest()
}

func (s *ServerSuite) TestRunServer_APIConnection() {
	ctx, cancel := newContext()

	monkey.Patch(gateway.GetDefaultLanListeningAddress, func(current, gateway string) (string, []int64) {
		return "127.0.0.1", nil
	})

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

	conn, err := openQuicSession_test("127.0.0.1", 9090)
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

	expectedRecv := `HTTP\/1\.1 200 OK
Connection: close
Content-Type: application\/json
Vary: Origin
Date: .+ GMT
Content-Length: \d+

{"address":"[^"]+","port":\d+,"serverversion":"[^"]+","total_connections":\d+}`

	matchStr := strings.ReplaceAll(string(receiveData[:recv]), "\r", "")

	stream.AbortWrite(0)
	stream.AbortRead(0)
	stream.Close()

	cancel()

	wg.Wait()

	re := regexp.MustCompile(expectedRecv)
	assert.True(s.T(), re.MatchString(matchStr))
}

func (s *ServerSuite) TestPerformanceWatcher_Panic() {
	var prevStats = api.Statistics
	api.Statistics = nil
	defer func() {
		api.Statistics = prevStats
	}()

	assert.NotPanics(s.T(), func() {
		ctx, cancel := newContext()
		performanceWatcher(ctx, cancel)
	})
}

func (s *ServerSuite) TestListenQuicSession_Panic() {
	quicProvider = fakeBackend
	ctx, cancel := newContext()
	listenQuicSession(ctx, cancel, "127.0.0.1", 9443)
}
