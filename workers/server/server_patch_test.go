//go:build !arm64

package server

import (
	"crypto/tls"
	"errors"
	"github.com/Project-Faster/monkey"
	"github.com/Project-Faster/qpep/api"
	"github.com/Project-Faster/qpep/shared"
	"github.com/Project-Faster/quic-go"
	"github.com/stretchr/testify/assert"
	"net"
	"regexp"
	"strings"
	"sync"
)

func (s *ServerSuite) TestRunServer_BadListener() {
	monkey.Patch(quic.ListenAddr, func(string, *tls.Config, *quic.Config) (quic.Listener, error) {
		return nil, errors.New("test-error")
	})

	s.runServerTest()
}

func (s *ServerSuite) TestRunServer_APIConnection() {
	monkey.Patch(shared.GetDefaultLanListeningAddress, func(current, gateway string) (string, []int64) {
		return "127.0.0.1", nil
	})

	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		RunServer(s.ctx, s.cancel)
	}()
	go func() {
		defer wg.Done()
		api.RunServer(s.ctx, s.cancel, false)
	}()

	addr, _ := shared.GetDefaultLanListeningAddress("127.0.0.1", "")

	conn, err := openQuicSession_test("127.0.0.1", 9090)
	assert.Nil(s.T(), err)

	stream, err := conn.OpenStream(s.ctx)
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

	s.cancel()

	wg.Wait()

	re := regexp.MustCompile(expectedRecv)
	assert.True(s.T(), re.MatchString(matchStr))
}
