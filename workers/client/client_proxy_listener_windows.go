//go:build windows

package client

import (
	"github.com/Project-Faster/qpep/shared/errors"
	"net"
)

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

// Accept method accepts the connections from generic connection types
func (listener *ClientProxyListener) Accept() (net.Conn, error) {
	conn, err := listener.AcceptTProxy()
	if err == nil {
		listener.internalConn = conn
	}
	return conn, err
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
