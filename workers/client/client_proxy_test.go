package client

import (
	"fmt"
	"github.com/Project-Faster/monkey"
	"github.com/Project-Faster/qpep/shared/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"net"
	"sync"
	"testing"
	"time"
)

func TestClientProxyListenerSuite(t *testing.T) {
	var q ClientProxyListenerSuite
	q.testLocalListenPort = 9990

	suite.Run(t, &q)
}

type ClientProxyListenerSuite struct {
	suite.Suite

	testLocalListenPort int
}

func (s *ClientProxyListenerSuite) BeforeTest(_, testName string) {
	s.testLocalListenPort++
}

func (s *ClientProxyListenerSuite) AfterTest(_, testName string) {
	monkey.UnpatchAll()
}

func (s *ClientProxyListenerSuite) TestNewClientProxyListener() {
	listener, err := NewClientProxyListener("tcp", &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: s.testLocalListenPort,
	})

	assert.Nil(s.T(), err)
	assert.NotNil(s.T(), listener)
	assert.NotNil(s.T(), listener.(*ClientProxyListener).base)

	assert.NotNil(s.T(), listener.Addr())
	assert.Equal(s.T(), fmt.Sprintf("127.0.0.1:%d", s.testLocalListenPort), listener.Addr().String())

	assert.Nil(s.T(), listener.Close())
}

func (s *ClientProxyListenerSuite) TestNewClientProxyListener_FailListen() {
	listener, err := NewClientProxyListener("tcp", &net.TCPAddr{
		Port: 99999,
	})

	assert.Nil(s.T(), listener)
	assert.NotNil(s.T(), err)
}

func (s *ClientProxyListenerSuite) TestProxyListener_AddrNil() {
	listener := &ClientProxyListener{}
	assert.Nil(s.T(), listener.Addr())
}

func (s *ClientProxyListenerSuite) TestProxyListener_CloseNil() {
	listener := &ClientProxyListener{}
	assert.Nil(s.T(), listener.Close())
}

func (s *ClientProxyListenerSuite) TestProxyListener_AcceptNil() {
	listener := &ClientProxyListener{}

	conn, err := listener.Accept()
	assert.Equal(s.T(), errors.ErrFailed, err)
	assert.Nil(s.T(), conn)
}

func (s *ClientProxyListenerSuite) TestProxyListener_AcceptConn() {
	listener, err := NewClientProxyListener("tcp", &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: s.testLocalListenPort,
	})
	assert.Nil(s.T(), err)
	assert.NotNil(s.T(), listener)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		conn, err := net.DialTCP("tcp", nil, &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: s.testLocalListenPort,
		})
		<-time.After(1 * time.Second)
		assert.Nil(s.T(), err)
		assert.NotNil(s.T(), conn)

		_ = conn.Close()
	}()

	conn, err := listener.Accept()
	assert.Nil(s.T(), err)
	assert.NotNil(s.T(), conn)

	assert.Nil(s.T(), listener.Close())
}
