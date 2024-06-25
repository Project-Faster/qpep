//go:build !no_quicgo_backend

package backend

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/Project-Faster/qpep/logger"
	"github.com/Project-Faster/qpep/shared"
	"github.com/Project-Faster/quic-go"
	"io/ioutil"
	"net"
	"strings"
	"time"
)

const (
	QUICGO_BACKEND = "quic-go"
	QUICGO_ALPN        = "qpep"
	QUICGO_DEFAULT_CCA = "reno"
)

var qgoBackend QuicBackend = &quicGoBackend{}

func init() {
	Register(QUICGO_BACKEND, qgoBackend)
}

type quicGoBackend struct {
	connections []QuicBackendConnection
}

func (q *quicGoBackend) Dial(ctx context.Context, remoteAddress string, port int, clientCertPath string, ccAlgorithm string, ccSlowstartAlgo string, traceOn bool) (QuicBackendConnection, error) {
	quicConfig := qgoGetConfiguration()

	var err error
	var session quic.Connection

	tlsConf := loadTLSConfig(clientCertPath, "")
	gatewayPath := fmt.Sprintf("%s:%d", remoteAddress, port)

	session, err = quic.DialAddr(gatewayPath, tlsConf, quicConfig)
	if err != nil {
		logger.Error("Unable to Dial QUIC Session: %v\n", err)
		return nil, shared.ErrFailedGatewayConnect
	}

	sessionAdapter := &qgoConnectionAdapter{
		context:    ctx,
		connection: session,
	}

	q.connections = append(q.connections, sessionAdapter)
	return sessionAdapter, nil
}

func (q *quicGoBackend) Listen(ctx context.Context, address string, port int, serverCertPath string, serverKeyPath string, ccAlgorithm string, ccSlowstartAlgo string, traceOn bool) (QuicBackendConnection, error) {
	quicConfig := qgoGetConfiguration()

	tlsConf := loadTLSConfig(serverCertPath, serverKeyPath)

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
		MaxIncomingStreams:      1024,
		DisablePathMTUDiscovery: true,
		MaxIdleTimeout:          3 * time.Second,

		HandshakeIdleTimeout: shared.GetScaledTimeout(10, time.Second),
		KeepAlivePeriod:      0,

		EnableDatagrams: false,
	}
}

type qgoConnectionAdapter struct {
	context    context.Context
	listener   quic.Listener
	connection quic.Connection

	streams []quic.Stream
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
			streams:    make([]quic.Stream, 0, 32),
		}
		return cNew, nil
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *qgoConnectionAdapter) AcceptStream(ctx context.Context) (QuicBackendStream, error) {
	if c.connection != nil {
		stream, err := c.connection.AcceptStream(ctx)
		if stream != nil {
			c.streams = append(c.streams, stream)
		}
		return &qgoStreamAdapter{
			Stream: stream,
		}, err
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *qgoConnectionAdapter) Close(code int, message string) error {
	defer func() {
		c.connection = nil
		c.listener = nil
		c.streams = nil
	}()
	if c.connection != nil {
		for _, st := range c.streams {
			st.CancelRead(quic.StreamErrorCode(0))
			st.CancelWrite(quic.StreamErrorCode(0))
			_ = st.Close()
		}
		return c.connection.CloseWithError(quic.ApplicationErrorCode(code), message)
	}
	if c.listener != nil {
		return c.listener.Close()
	}
	return nil
}

func (c *qgoConnectionAdapter) IsClosed() bool {
	return c.connection == nil && c.listener == nil
}

var _ QuicBackendConnection = &qgoConnectionAdapter{}

type qgoStreamAdapter struct {
	quic.Stream

	id *uint64

	closedRead  bool
	closedWrite bool
}

func (stream *qgoStreamAdapter) AbortRead(code uint64) {
	stream.CancelRead(quic.StreamErrorCode(code))
	stream.closedRead = true
}

func (stream *qgoStreamAdapter) AbortWrite(code uint64) {
	stream.CancelWrite(quic.StreamErrorCode(code))
	stream.closedWrite = true
}

func (stream *qgoStreamAdapter) Sync() bool {
	return stream.IsClosed()
}

func (stream *qgoStreamAdapter) ID() uint64 {
	if stream.id != nil {
		return *stream.id
	}
	var sendStream quic.SendStream = stream
	if sendStream != nil {
		stream.id = new(uint64)
		*stream.id = uint64(sendStream.StreamID())
		return *stream.id
	}
	var recvStream quic.ReceiveStream = stream
	if recvStream != nil {
		stream.id = new(uint64)
		*stream.id = uint64(recvStream.StreamID())
		return *stream.id
	}
	return 0
}

func (stream *qgoStreamAdapter) IsClosed() bool {
	return false // stream.closedRead || stream.closedWrite
}

var _ QuicBackendStream = &qgoStreamAdapter{}
