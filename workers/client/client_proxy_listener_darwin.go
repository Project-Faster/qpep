//go:build darwin

package client

import (
	"fmt"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// NewClientProxyListener method instantiates a new ClientProxyListener on a tcp address base listener
func NewClientProxyListener(network string, laddr *net.TCPAddr) (net.Listener, error) {
	//Dial basic TCP listener
	listener, err := net.ListenTCP(network, laddr)
	if err != nil {
		return nil, err
	}

	//Find associated file descriptor for listener to set socket options on
	fileDescriptorSource, err := listener.File()
	if err != nil {
		return nil, &net.OpError{Op: "ClientListener", Net: network, Source: nil, Addr: laddr, Err: fmt.Errorf("get file descriptor: %s", err)}
	}
	defer fileDescriptorSource.Close()

	_ = syscall.SetsockoptInt(int(fileDescriptorSource.Fd()), syscall.IPPROTO_TCP, unix.TCP_FASTOPEN, 1)

	//return a derived TCP listener object with TCProxy support
	return &ClientProxyListener{base: listener}, nil
}
