//go:build windows

package client

import (
	"github.com/parvit/qpep/shared/errors"
	"net"
)

// ClientProxyListener implements the local listener for diverted connections
type ClientProxyListener struct {
	base net.Listener
}

// Accept method accepts the connections from generic connection types
func (listener *ClientProxyListener) Accept() (net.Conn, error) {
	if listener.base == nil {
		return nil, errors.ErrFailed
	}
	tcpConn, err := listener.base.(*net.TCPListener).AcceptTCP()
	if err != nil {
		return nil, err
	}
	return tcpConn, nil
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

// NewClientProxyListener method instantiates a new ClientProxyListener on a tcp address base listener
func NewClientProxyListener(network string, laddr *net.TCPAddr) (net.Listener, error) {
	//Dial basic TCP listener
	listener, err := net.ListenTCP(network, laddr)
	if err != nil {
		return nil, err
	}

	//return a derived TCP listener object with TCProxy support
	return &ClientProxyListener{base: listener}, nil
}
