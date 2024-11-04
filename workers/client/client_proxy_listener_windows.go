//go:build windows

package client

import (
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
