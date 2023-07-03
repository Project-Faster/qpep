package backend

import (
	"context"
	"io"
	"net"
	"time"
)

type QuicBackend interface {
	Dial(ctx context.Context, remoteAddress string, port int) (QuicBackendConnection, error)
	Listen(ctx context.Context, address string, port int) (QuicBackendConnection, error)
	Close() error
}

type QuicBackendConnection interface {
	// LocalAddr returns the local address.
	LocalAddr() net.Addr
	// RemoteAddr returns the address of the peer.
	RemoteAddr() net.Addr
	OpenStream(context.Context) (QuicBackendStream, error)
	AcceptStream(context.Context) (QuicBackendStream, error)
	AcceptConnection(context.Context) (QuicBackendConnection, error)
	Close(code int, message string) error
}

type QuicBackendStream interface {
	io.Reader
	io.Writer
	io.Closer

	ID() uint64

	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}
