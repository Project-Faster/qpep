//go:build !no_quicgo_backend

package backend

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/Project-Faster/quic-go"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"net"
	"time"
)

const (
	QUICGO_BACKEND = "quic-go"
)

var qgoBackend QuicBackend = &quicGoBackend{}

func init() {
	Register(QUICGO_BACKEND, qgoBackend)
}

type quicGoBackend struct {
	connections []QuicBackendConnection
}

func (q *quicGoBackend) Dial(ctx context.Context, destination string, port int) (QuicBackendConnection, error) {
	quicConfig := qgoGetConfiguration()

	var err error
	var session quic.Connection
	tlsConf := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"qpep"}}
	gatewayPath := fmt.Sprintf("%s:%d", destination, port)

	logger.Info("== Dialing QUIC Session: %s ==\n", gatewayPath)
	session, err = quic.DialAddr(gatewayPath, tlsConf, quicConfig)
	if err != nil {
		logger.Error("Unable to Dial QUIC Session: %v\n", err)
		return nil, shared.ErrFailedGatewayConnect
	}

	sessionAdapter := &qgoConnectionAdapter{
		context:    ctx,
		connection: session,
	}

	logger.Info("== QUIC Session Dial ==\n")
	q.connections = append(q.connections, sessionAdapter)
	return sessionAdapter, nil
}

func (q *quicGoBackend) Listen(ctx context.Context, address string, port int) (QuicBackendConnection, error) {
	quicConfig := qgoGetConfiguration()

	tlsConf := generateTLSConfig("server")

	conn, err := quic.ListenAddr(fmt.Sprintf("%s:%d", address, port), tlsConf, quicConfig)
	if err != nil {
		logger.Error("Failed to listen on QUIC session: %v\n", err)
		return nil, shared.ErrFailedGatewayConnect
	}

	return &qgoConnectionAdapter{
		context:  ctx,
		listener: conn,
	}, err
}

func (q *quicGoBackend) Close() error {
	for _, conn := range q.connections {
		_ = conn.Close(0, "")
	}
	q.connections = nil
	logger.Info("== QUIC Session Closed ==\n")
	return nil
}

func qgoGetConfiguration() *quic.Config {
	return &quic.Config{
		MaxIncomingStreams:      16,
		DisablePathMTUDiscovery: false,

		HandshakeIdleTimeout: shared.GetScaledTimeout(10, time.Second),
		//KeepAlivePeriod:      1 * time.Second,

		EnableDatagrams: true,
	}
}

type qgoConnectionAdapter struct {
	context    context.Context
	listener   quic.Listener
	connection quic.Connection
}

func (c *qgoConnectionAdapter) LocalAddr() net.Addr {
	if c.connection != nil {
		return c.connection.LocalAddr()
	}
	if c.listener != nil {
		return c.listener.Addr()
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *qgoConnectionAdapter) RemoteAddr() net.Addr {
	if c.connection != nil {
		return c.connection.RemoteAddr()
	}
	if c.listener != nil {
		return c.listener.Addr()
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *qgoConnectionAdapter) AcceptStream(ctx context.Context) (QuicBackendStream, error) {
	if c.connection != nil {
		stream, err := c.connection.AcceptStream(ctx)
		return &qgoStreamAdapter{
			Stream: stream,
		}, err
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *qgoConnectionAdapter) OpenStream(ctx context.Context) (QuicBackendStream, error) {
	if c.connection != nil {
		stream, err := c.connection.OpenStreamSync(ctx)
		return &qgoStreamAdapter{
			Stream: stream,
		}, err
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *qgoConnectionAdapter) AcceptConnection(ctx context.Context) (QuicBackendConnection, error) {
	if c.listener != nil {
		conn, err := c.listener.Accept(ctx)
		if err != nil {
			return nil, err
		}
		cNew := &qgoConnectionAdapter{
			context:    ctx,
			listener:   c.listener,
			connection: conn,
		}
		return cNew, nil
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *qgoConnectionAdapter) Close(code int, message string) error {
	if c.connection != nil {
		return c.connection.CloseWithError(quic.ApplicationErrorCode(code), message)
	}
	if c.listener != nil {
		return c.listener.Close()
	}
	panic(shared.ErrInvalidBackendOperation)
}

var _ QuicBackendConnection = &qgoConnectionAdapter{}

type qgoStreamAdapter struct {
	quic.Stream
}

func (stream *qgoStreamAdapter) AbortRead(code uint64) {
	stream.CancelRead(quic.StreamErrorCode(code))
}

func (stream *qgoStreamAdapter) AbortWrite(code uint64) {
	stream.CancelWrite(quic.StreamErrorCode(code))
}

func (stream *qgoStreamAdapter) ID() uint64 {
	var sendStream quic.SendStream = stream
	if sendStream != nil {
		return uint64(sendStream.StreamID())
	}
	var recvStream quic.ReceiveStream = stream
	if recvStream != nil {
		return uint64(recvStream.StreamID())
	}
	return 0
}

var _ QuicBackendStream = &qgoStreamAdapter{}
