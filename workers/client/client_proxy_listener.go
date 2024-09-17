package client

import (
	"github.com/Project-Faster/qpep/shared/errors"
	"net"
)

// ClientProxyListener implements the local listener for diverted connections
type ClientProxyListener struct {
	base proxyInterface
}

type proxyInterface interface {
	AcceptTCP() (*net.TCPConn, error)
	Addr() net.Addr
	Close() error
}

// Addr method returns the listening address
func (listener *ClientProxyListener) Addr() net.Addr {
	if listener.base == nil {
		return nil
	}
	return listener.base.Addr()
}

// Close method close the listener
func (listener *ClientProxyListener) Close() error {
	if listener.base == nil {
		return nil
	}
	return listener.base.Close()
}

// Accept method accepts the connections from generic connection types
func (listener *ClientProxyListener) Accept() (net.Conn, error) {
	return listener.AcceptTProxy()
}

// AcceptTProxy method accepts the connections and casts those to a tcp connection type
func (listener *ClientProxyListener) AcceptTProxy() (*net.TCPConn, error) {
	if listener.base == nil {
		return nil, errors.ErrFailed
	}
	tcpConn, err := listener.base.AcceptTCP()
	if err != nil {
		return nil, err
	}
	return tcpConn, nil
}
