//go:build !no_quiclygo_backend

package backend

import (
	"context"
	"github.com/Project-Faster/quicly-go"
	"github.com/Project-Faster/quicly-go/quiclylib/errors"
	"github.com/Project-Faster/quicly-go/quiclylib/types"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"net"
)

func init() {
	Register(QUICLYGO_BACKEND, quiclyBackend)
}

const (
	QUICLYGO_BACKEND = "quicly-go"
)

var quiclyBackend QuicBackend = &quiclyGoBackend{}

type quiclyGoBackend struct {
	connections []QuicBackendConnection
	initialized bool
}

func (q *quiclyGoBackend) Dial(ctx context.Context, destination string, port int) (QuicBackendConnection, error) {
	if !q.initialized {
		_ = generateTLSConfig("client")

		quicConfig := quicly.Options{
			Logger:          logger.GetLogger(),
			CertificateFile: "client_cert.pem",
			CertificateKey:  "",
		}

		if err := quicly.Initialize(quicConfig); err != errors.QUICLY_OK {
			return nil, shared.ErrFailed
		}
		q.initialized = true
	}

	ipAddr := net.ParseIP(destination)

	remoteAddr := net.UDPAddr{
		IP:   ipAddr,
		Port: port,
	}

	logger.Info("== Dialing QUIC Session: %s ==\n", destination)

	session := quicly.Dial(&remoteAddr, types.Callbacks{
		OnConnectionOpen: func(connection types.Session) {
			logger.Info("OPEN: %v", connection)
		},
		OnConnectionClose: func(connection types.Session) {
			logger.Info("CLOSE: %v", connection)
		},
		OnStreamOpenCallback: func(stream types.Stream) {
			logger.Info(">> Callback open %d", stream.ID())
		},
		OnStreamCloseCallback: func(stream types.Stream, error int) {
			logger.Info(">> Callback close %d, error %d", stream.ID(), error)
		},
	}, ctx)

	if session == nil {
		logger.Error("Unable to Dial QUIC Session\n")
		return nil, shared.ErrFailedGatewayConnect
	}

	sessionAdapter := &connectionAdapter{
		context:    ctx,
		connection: session,
	}

	logger.Info("== QUIC Session Dial ==\n")
	q.connections = append(q.connections, sessionAdapter)
	return sessionAdapter, nil
}

func (q *quiclyGoBackend) Listen(ctx context.Context, address string, port int) (QuicBackendConnection, error) {
	if !q.initialized {
		_ = generateTLSConfig("server")

		quicConfig := quicly.Options{
			Logger:          logger.GetLogger(),
			CertificateFile: "server_cert.pem",
			CertificateKey:  "server_key.pem",
		}

		if err := quicly.Initialize(quicConfig); err != errors.QUICLY_OK {
			return nil, shared.ErrFailed
		}
		q.initialized = true
	}

	ipAddr := net.ParseIP(address)

	localAddr := net.UDPAddr{
		IP:   ipAddr,
		Port: port,
	}

	conn := quicly.Listen(&localAddr, types.Callbacks{
		OnConnectionOpen: func(conn types.Session) {
			logger.Info("OnStart")
		},
		OnConnectionClose: func(conn types.Session) {
			logger.Info("OnClose")
		},
		OnStreamOpenCallback: func(stream types.Stream) {
			logger.Info(">> Callback open %d", stream.ID())
		},
		OnStreamCloseCallback: func(stream types.Stream, error int) {
			logger.Info(">> Callback close %d, error %d", stream.ID(), error)
		},
	}, ctx)

	return &connectionAdapter{
		context:    ctx,
		connection: conn,
	}, nil
}

func (q *quiclyGoBackend) Close() error {
	if !q.initialized {
		logger.Error("Backend was not initialized")
		return shared.ErrFailed
	}
	for _, conn := range q.connections {
		_ = conn.Close(0, "")
	}
	q.connections = nil
	q.initialized = false
	logger.Info("== QUIC Session Closed ==")
	return nil
}

type connectionAdapter struct {
	context    context.Context
	connection types.Session
}

func (c *connectionAdapter) LocalAddr() net.Addr {
	if c.connection != nil {
		return c.connection.Addr()
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *connectionAdapter) RemoteAddr() net.Addr {
	if c.connection != nil {
		return c.connection.Addr()
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *connectionAdapter) AcceptStream(ctx context.Context) (QuicBackendStream, error) {
	if c.connection != nil {
		stream, err := c.connection.Accept()
		return &streamAdapter{
			Conn: stream,
			id:   c.connection.ID(),
		}, err
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *connectionAdapter) OpenStream(ctx context.Context) (QuicBackendStream, error) {
	if c.connection != nil {
		stream := c.connection.OpenStream()
		return &streamAdapter{
			Conn: stream,
			id:   stream.ID(),
		}, nil
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *connectionAdapter) AcceptConnection(ctx context.Context) (QuicBackendConnection, error) {
	panic(shared.ErrInvalidBackendOperation)
}

func (c *connectionAdapter) Close(code int, message string) error {
	if c.connection != nil {
		return c.connection.Close()
	}
	panic(shared.ErrInvalidBackendOperation)
}

var _ QuicBackendConnection = &connectionAdapter{}

type streamAdapter struct {
	net.Conn

	id uint64
}

func (s *streamAdapter) ID() uint64 {
	return s.id
}

var _ QuicBackendStream = &streamAdapter{}
