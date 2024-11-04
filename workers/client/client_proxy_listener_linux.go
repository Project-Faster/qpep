//go:build linux

package client

import (
	stderr "errors"
	"fmt"
	"net"
	"syscall"
	"time"

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

	//Make the port transparent so the gateway can see the real origin IP address (invisible proxy within satellite environment)
	_ = syscall.SetsockoptInt(int(fileDescriptorSource.Fd()), syscall.SOL_IP, syscall.IP_TRANSPARENT, 1)
	_ = syscall.SetsockoptInt(int(fileDescriptorSource.Fd()), syscall.SOL_TCP, unix.TCP_FASTOPEN, 1)

	//return a derived TCP listener object with TCProxy support
	return &ClientProxyListener{base: listener}, nil
}

type wrappedTcpConn struct {
	internal   *net.TCPConn
	remoteAddr *net.TCPAddr
}

func (w *wrappedTcpConn) RemoteAddr() net.Addr {
	if w.remoteAddr == nil {
		w.remoteAddr, w.internal, _ = getOriginalDst(w.internal)
	}
	return w.remoteAddr
}

// get the original destination for the socket when redirect by linux iptables
const (
	SO_ORIGINAL_DST = 80
)

func getOriginalDst(clientConn *net.TCPConn) (*net.TCPAddr, *net.TCPConn, error) {
	if clientConn == nil {
		return nil, nil, stderr.New("ERR: clientConn is nil")
	}

	// test if the underlying fd is nil
	remoteAddr := clientConn.RemoteAddr()
	if remoteAddr == nil {
		return nil, nil, stderr.New("ERR: clientConn.fd is nil")
	}

	fmt.Printf(">> %v\n", clientConn.RemoteAddr())

	// net.TCPConn.File() will cause the receiver's (clientConn) socket to be placed in blocking mode.
	// The workaround is to take the File returned by .File(), do getsockopt() to get the original
	// destination, then create a new *net.TCPConn by calling net.Conn.FileConn().  The new TCPConn
	// will be in non-blocking mode.
	clientConnFile, err := clientConn.File()
	if err != nil {
		return nil, nil, err
	}

	clientConn.Close()

	// Get original destination
	addr, err := syscall.GetsockoptIPv6Mreq(int(clientConnFile.Fd()), syscall.IPPROTO_IP, SO_ORIGINAL_DST)
	if err != nil {
		return nil, nil, err
	}
	newConn, err := net.FileConn(clientConnFile)
	if err != nil {
		return nil, nil, err
	}

	var newTCPConn *net.TCPConn = nil
	if _, ok := newConn.(*net.TCPConn); ok {
		newTCPConn = newConn.(*net.TCPConn)
		clientConnFile.Close()
	} else {
		errmsg := fmt.Sprintf("ERR: newConn is not a *net.TCPConn, instead it is: %T (%v)", newConn, newConn)
		return nil, nil, stderr.New(errmsg)
	}

	// attention: IPv4 only!!!
	var ipAddr = net.IPv4(addr.Multiaddr[4], addr.Multiaddr[5], addr.Multiaddr[6], addr.Multiaddr[7])
	var port = uint16(addr.Multiaddr[2])<<8 + uint16(addr.Multiaddr[3])

	newAddr := &net.TCPAddr{IP: ipAddr, Port: int(port)}

	return newAddr, newTCPConn, nil
}

func (w *wrappedTcpConn) Read(b []byte) (n int, err error) {
	return w.internal.Read(b)
}

func (w *wrappedTcpConn) Write(b []byte) (n int, err error) {
	return w.internal.Write(b)
}

func (w *wrappedTcpConn) Close() error {
	return w.internal.Close()
}

func (w *wrappedTcpConn) LocalAddr() net.Addr {
	return w.internal.LocalAddr()
}

func (w *wrappedTcpConn) SetDeadline(t time.Time) error {
	return w.internal.SetDeadline(t)
}

func (w *wrappedTcpConn) SetReadDeadline(t time.Time) error {
	return w.internal.SetReadDeadline(t)
}

func (w *wrappedTcpConn) SetWriteDeadline(t time.Time) error {
	return w.internal.SetWriteDeadline(t)
}

var _ net.Conn = (*wrappedTcpConn)(nil)
