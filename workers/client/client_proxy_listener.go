package client

import (
	"net"
)

// ClientProxyListener implements the local listener for diverted connections
type ClientProxyListener struct {
	base proxyInterface

	internalConn *net.TCPConn
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
