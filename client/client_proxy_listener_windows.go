//go:build windows

package client

import (
	"net"
)

// ClientProxyListener implements the local listener for diverted connections
type ClientProxyListener struct {
	base net.Listener
}

// Accept method accepts the connections from generic connection types
func (listener *ClientProxyListener) Accept() (net.Conn, error) {
	return listener.AcceptTProxy()
}

// AcceptTProxy method accepts the connections and casts those to a tcp connection type
func (listener *ClientProxyListener) AcceptTProxy() (*net.TCPConn, error) {
	tcpConn, err := listener.base.(*net.TCPListener).AcceptTCP()
	if err != nil {
		return nil, err
	}
	return tcpConn, nil
	//return &ProxyConn{TCPConn: tcpConn}, nil
}

// Addr method returns the listening address
func (listener *ClientProxyListener) Addr() net.Addr {
	return listener.base.Addr()
}

// Addr method close the listener
func (listener *ClientProxyListener) Close() error {
	return listener.base.Close()
}

// NewClientProxyListener method instantiates a new ClientProxyListener on a tcp address base listener
func NewClientProxyListener(network string, laddr *net.TCPAddr) (net.Listener, error) {
	//Open basic TCP listener
	listener, err := net.ListenTCP(network, laddr)
	if err != nil {
		return nil, err
	}

	//return a derived TCP listener object with TCProxy support
	return &ClientProxyListener{base: listener}, nil
}
