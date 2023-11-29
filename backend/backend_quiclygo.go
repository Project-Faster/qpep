//go:build !no_quiclygo_backend

package backend

import (
	"context"
	"fmt"
	"github.com/Project-Faster/quicly-go"
	"github.com/Project-Faster/quicly-go/quiclylib/errors"
	"github.com/Project-Faster/quicly-go/quiclylib/types"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"net"
	"sync"
)

func init() {
	Register(QUICLYGO_BACKEND, quiclyBackend)
}

const (
	QUICLYGO_BACKEND = "quicly-go"
)

var (
	quiclyBackend QuicBackend = &quiclyGoBackend{
		connections: make(map[string]QuicBackendConnection),
		initialized: false,
	}
)

type quiclyGoBackend struct {
	initialized bool

	opLock      sync.RWMutex
	connections map[string]QuicBackendConnection
}

func (q *quiclyGoBackend) getListener(destination string) QuicBackendConnection {
	q.opLock.Lock()
	defer q.opLock.Unlock()

	return q.connections[destination]
}

func (q *quiclyGoBackend) setListener(destination string, conn QuicBackendConnection) {
	q.opLock.Lock()
	defer q.opLock.Unlock()

	q.connections[destination] = conn
}

func (q *quiclyGoBackend) Dial(ctx context.Context, destination string, port int) (QuicBackendConnection, error) {
	if !q.initialized {
		_ = generateTLSConfig("client")

		quicConfig := quicly.Options{
			Logger:              logger.GetLogger(),
			CertificateFile:     "client_cert.pem",
			CertificateKey:      "",
			ApplicationProtocol: "qpep_quicly",
			IdleTimeoutMs:       3 * 1000,
			CongestionAlgorithm: "search",
		}

		if err := quicly.Initialize(quicConfig); err != errors.QUICLY_OK {
			return nil, shared.ErrFailed
		}
		q.initialized = true
	}

	remoteAddress := fmt.Sprintf("%s:%d", destination, port)

	conn := q.getListener(remoteAddress)
	if conn != nil {
		return conn, nil
	}

	ipAddr := net.ParseIP(destination)

	remoteAddr := net.UDPAddr{
		IP:   ipAddr,
		Port: port,
	}

	session := quicly.Dial(&remoteAddr, types.Callbacks{
		OnConnectionOpen: func(connection types.Session) {
			logger.Info("OPEN: %v", connection)
		},
		OnConnectionClose: func(connection types.Session) {
			logger.Info("CLOSE: %v", connection)

			q.setListener(remoteAddress, nil)
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

	sessionAdapter := &cliConnectionAdapter{
		context:    ctx,
		connection: session,
	}

	logger.Info("== QUIC Session Dial ==\n")
	q.setListener(destination, sessionAdapter)

	return sessionAdapter, nil
}

func (q *quiclyGoBackend) Listen(ctx context.Context, address string, port int) (QuicBackendConnection, error) {
	if !q.initialized {
		_ = generateTLSConfig("server")

		quicConfig := quicly.Options{
			Logger:              logger.GetLogger(),
			CertificateFile:     "server_cert.pem",
			CertificateKey:      "server_key.pem",
			ApplicationProtocol: "qpep_quicly",
			IdleTimeoutMs:       3 * 1000,
			CongestionAlgorithm: "search",
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

	session := quicly.Listen(&localAddr, types.Callbacks{
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

	return &srvListenerAdapter{
		context:  ctx,
		listener: session,
		backend:  q,
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

// -------------------------- //
type srvListenerAdapter struct {
	context  context.Context
	backend  *quiclyGoBackend
	listener types.ServerSession
}

func (c *srvListenerAdapter) LocalAddr() net.Addr {
	if c.listener != nil {
		return c.listener.Addr()
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *srvListenerAdapter) RemoteAddr() net.Addr {
	if c.listener != nil {
		return c.listener.Addr()
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *srvListenerAdapter) AcceptConnection(ctx context.Context) (QuicBackendConnection, error) {
	if c.listener != nil {
		conn, err := c.listener.Accept()
		if err != nil {
			return nil, err
		}
		return &srvConnectionAdapter{
			context:    ctx,
			backend:    c.backend,
			connection: conn,
		}, nil
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *srvListenerAdapter) AcceptStream(ctx context.Context) (QuicBackendStream, error) {
	panic(shared.ErrInvalidBackendOperation)
}

func (c *srvListenerAdapter) OpenStream(ctx context.Context) (QuicBackendStream, error) {
	panic(shared.ErrInvalidBackendOperation)
}

func (c *srvListenerAdapter) Close(code int, message string) error {
	if c.listener != nil {
		return c.listener.Close()
	}
	panic(shared.ErrInvalidBackendOperation)
}

// -------------------------- //

type srvConnectionAdapter struct {
	context    context.Context
	backend    *quiclyGoBackend
	connection types.ServerConnection
}

func (c *srvConnectionAdapter) LocalAddr() net.Addr {
	if c.connection != nil {
		return c.connection.Addr()
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *srvConnectionAdapter) RemoteAddr() net.Addr {
	return nil
}

func (c *srvConnectionAdapter) AcceptConnection(ctx context.Context) (QuicBackendConnection, error) {
	panic(shared.ErrInvalidBackendOperation)
}

func (c *srvConnectionAdapter) AcceptStream(ctx context.Context) (QuicBackendStream, error) {
	if c.connection != nil {
		stream, err := c.connection.Accept()
		return &streamAdapter{
			Conn: stream,
			id:   stream.ID(),
		}, err
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *srvConnectionAdapter) OpenStream(ctx context.Context) (QuicBackendStream, error) {
	panic(shared.ErrInvalidBackendOperation)
}

func (c *srvConnectionAdapter) Close(code int, message string) error {
	if c.connection != nil {
		return c.connection.Close()
	}
	panic(shared.ErrInvalidBackendOperation)
}

// -------------------------- //

var _ QuicBackendConnection = &srvConnectionAdapter{}

type cliConnectionAdapter struct {
	context    context.Context
	connection types.ClientSession
	backend    *quiclyGoBackend
}

func (c *cliConnectionAdapter) LocalAddr() net.Addr {
	return nil
}

func (c *cliConnectionAdapter) RemoteAddr() net.Addr {
	if c.connection != nil {
		return c.connection.Addr()
	}
	return nil
}

func (c *cliConnectionAdapter) AcceptStream(ctx context.Context) (QuicBackendStream, error) {
	panic(shared.ErrInvalidBackendOperation)
}

func (c *cliConnectionAdapter) AcceptConnection(ctx context.Context) (QuicBackendConnection, error) {
	panic(shared.ErrInvalidBackendOperation)
}

func (c *cliConnectionAdapter) Close(code int, message string) error {
	if c.connection != nil {
		return c.connection.Close()
	}
	return nil
}

func (c *cliConnectionAdapter) OpenStream(ctx context.Context) (QuicBackendStream, error) {
	if c.connection != nil {
		stream := c.connection.OpenStream()
		if stream == nil {
			return nil, shared.ErrFailed
		}

		return &streamAdapter{
			Conn: stream,
			id:   stream.ID(),
		}, nil
	}
	panic(shared.ErrInvalidBackendOperation)
}

var _ QuicBackendConnection = &cliConnectionAdapter{}

// -------------------------- //
type streamAdapter struct {
	net.Conn

	id uint64
}

func (s *streamAdapter) ID() uint64 {
	return s.id
}

func (s *streamAdapter) AbortRead(code uint64) {
	// no-op
}

func (s *streamAdapter) AbortWrite(code uint64) {
	// no-op
}

var _ QuicBackendStream = &streamAdapter{}
