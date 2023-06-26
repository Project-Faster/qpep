package backend

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"time"
)

type QpepBackend interface {
	Open(ctx context.Context, remoteAddress string) (QpepConnection, error)
	Listen(addr string, tlsConf *tls.Config) (QpepConnection, error)
	Close() error
}

type QpepConnection interface {
	// LocalAddr returns the local address.
	LocalAddr() net.Addr
	// RemoteAddr returns the address of the peer.
	RemoteAddr() net.Addr
	AcceptStream(context.Context) (QpepStream, error)
	AcceptConnection(context.Context) (QpepConnection, error)
	Close(code int, message string) error
}

type QpepStream interface {
	io.Reader
	io.Writer
	io.Closer

	ID() int64

	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}
