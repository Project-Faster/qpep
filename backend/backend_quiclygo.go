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
	"os"
	"strings"
	"sync"
)

func init() {
	Register(QUICLYGO_BACKEND, quiclyBackend)
}

const (
	QUICLYGO_BACKEND     = "quicly-go"
	QUICLYGO_ALPN        = "qpep_quicly"
	QUICLYGO_DEFAULT_CCA = "reno"
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
	return q.connections[destination]
}

func (q *quiclyGoBackend) setListener(destination string, conn QuicBackendConnection) {
	q.connections[destination] = conn
}

func (q *quiclyGoBackend) init(isClient, traceOn bool, certPath, certKeyPath, ccAlgorithm, ccSlowstartAlgo string) error {
	q.opLock.Lock()
	defer q.opLock.Unlock()

	if q.initialized {
		return nil
	}
	lg := logger.GetLogger()

	ccAlgorithm = strings.TrimSpace(ccAlgorithm)
	if len(ccAlgorithm) == 0 {
		ccAlgorithm = QUICLYGO_DEFAULT_CCA
	}

	if len(certKeyPath) > 0 {
		if _, err := os.Stat(certPath); err != nil {
			generateTLSConfig(certPath, certKeyPath)
		}
	}

	quicConfig := quicly.Options{
		Logger:               lg,
		IsClient:             isClient,
		CertificateFile:      certPath,
		CertificateKey:       certKeyPath,
		ApplicationProtocol:  QUICLYGO_ALPN,
		IdleTimeoutMs:        30 * 1000,
		CongestionAlgorithm:  ccAlgorithm,
		CCSlowstartAlgorithm: ccSlowstartAlgo,
		TraceQuicly:          traceOn,
	}

	if err := quicly.Initialize(quicConfig); err != errors.QUICLY_OK {
		return shared.ErrFailed
	}
	q.initialized = true

	return nil
}

func (q *quiclyGoBackend) Dial(ctx context.Context, remoteAddress string, port int, clientCertPath string,
	ccAlgorithm string, ccSlowstartAlgo string, traceOn bool) (QuicBackendConnection, error) {

	if err := q.init(true, traceOn, clientCertPath, "", ccAlgorithm, ccSlowstartAlgo); err != nil {
		return nil, err
	}

	q.opLock.Lock()
	conn := q.getListener(remoteAddress)
	if conn != nil {
		if !conn.IsClosed() {
			q.opLock.Unlock()
			return conn, nil
		}
		q.setListener(remoteAddress, nil)
	}

	ipAddr := net.ParseIP(remoteAddress)

	remoteAddr := net.UDPAddr{
		IP:   ipAddr,
		Port: port,
	}

	session := quicly.Dial(&remoteAddr, types.Callbacks{
		OnConnectionOpen: func(connection types.Session) {
			logger.Info("OPEN [%v]: %v", remoteAddress, &connection)
		},
		OnConnectionClose: func(connection types.Session) {
			logger.Info("CLOSE [%v]: %v", remoteAddress, &connection)

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
		q.opLock.Unlock()
		return nil, shared.ErrFailedGatewayConnect
	}

	sessionAdapter := &cliConnectionAdapter{
		context:    ctx,
		connection: session,
	}

	logger.Info("== QUIC Session Dial ==\n")
	q.setListener(remoteAddress, sessionAdapter)
	q.opLock.Unlock()

	return sessionAdapter, nil
}

func (q *quiclyGoBackend) Listen(ctx context.Context, address string, port int, serverCertPath string, serverKeyPath string,
	ccAlgorithm string, ccSlowstartAlgo string, traceOn bool) (QuicBackendConnection, error) {

	if err := q.init(false, traceOn, serverCertPath, serverKeyPath, ccAlgorithm, ccSlowstartAlgo); err != nil {
		return nil, err
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

	conn := &srvListenerAdapter{
		context:  ctx,
		listener: session,
		backend:  q,
	}
	return conn, nil
}

func (q *quiclyGoBackend) Close() error {
	if !q.initialized {
		logger.Error("Backend was not initialized")
		return shared.ErrFailed
	}
	q.opLock.Lock()
	defer q.opLock.Unlock()
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
	return nil
}

func (c *srvListenerAdapter) RemoteAddr() net.Addr {
	if c.listener != nil {
		return c.listener.Addr()
	}
	return nil
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
	return nil, shared.ErrInvalidBackendOperation
}

func (c *srvListenerAdapter) AcceptStream(ctx context.Context) (QuicBackendStream, error) {
	panic(shared.ErrInvalidBackendOperation)
}

func (c *srvListenerAdapter) OpenStream(ctx context.Context) (QuicBackendStream, error) {
	panic(shared.ErrInvalidBackendOperation)
}

func (c *srvListenerAdapter) Close(code int, message string) error {
	defer func() {
		c.listener = nil
	}()
	if c.listener != nil {
		return c.listener.Close()
	}
	return nil
}

func (c *srvListenerAdapter) IsClosed() bool {
	if c.listener != nil {
		return c.listener.IsClosed()
	}
	return true
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
	return nil
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
		if stream == nil {
			return nil, err
		}
		return &streamAdapter{
			Conn:      stream,
			rawStream: stream,
			id:        stream.ID(),
		}, err
	}
	return nil, shared.ErrInvalidBackendOperation
}

func (c *srvConnectionAdapter) OpenStream(ctx context.Context) (QuicBackendStream, error) {
	panic(shared.ErrInvalidBackendOperation)
}

func (c *srvConnectionAdapter) Close(code int, message string) error {
	if c.connection != nil {
		defer func() {
			c.connection = nil
		}()
		return c.connection.Close()
	}
	return nil
}

func (c *srvConnectionAdapter) IsClosed() bool {
	if c.connection != nil {
		return c.connection.IsClosed()
	}
	return true
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
		defer func() {
			c.connection = nil
		}()
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
			Conn:      stream,
			rawStream: stream,
			id:        stream.ID(),
		}, nil
	}
	return nil, shared.ErrFailed
}

func (c *cliConnectionAdapter) IsClosed() bool {
	if c.connection != nil {
		return c.connection.IsClosed()
	}
	return true
}

var _ QuicBackendConnection = &cliConnectionAdapter{}

// -------------------------- //
type streamAdapter struct {
	net.Conn

	id        uint64
	rawStream types.Stream
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

func (s *streamAdapter) Sync() bool {
	return s.rawStream.Sync()
}

func (s *streamAdapter) IsClosed() bool {
	if s.rawStream != nil {
		return s.rawStream.IsClosed()
	}
	return true
}

var _ QuicBackendStream = &streamAdapter{}
