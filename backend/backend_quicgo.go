//go:build !go_quicgo

package backend

import (
	"context"
	"crypto/tls"
	"github.com/Project-Faster/quic-go"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"net"
	"time"
)

const (
	QUICGO_BACKEND = "quic-go"
)

var backendInstance QpepBackend = &quicGoBackend{}

func init() {
	Register(QUICGO_BACKEND, backendInstance)
}

type quicGoBackend struct {
	connections []QpepConnection
}

func (q *quicGoBackend) Open(ctx context.Context, remoteDestination string) (QpepConnection, error) {
	quicConfig := getConfiguration()

	var err error
	var session quic.Connection
	tlsConf := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"qpep"}}
	gatewayPath := remoteDestination

	logger.Info("== Dialing QUIC Session: %s ==\n", gatewayPath)
	session, err = quic.DialAddr(gatewayPath, tlsConf, quicConfig)
	if err != nil {
		logger.Error("Unable to Open QUIC Session: %v\n", err)
		return nil, shared.ErrFailedGatewayConnect
	}

	sessionAdapter := &connectionAdapter{
		context:    ctx,
		connection: session,
	}

	logger.Info("== QUIC Session Open ==\n")
	q.connections = append(q.connections, sessionAdapter)
	return sessionAdapter, nil
}

func (q *quicGoBackend) Listen(addr string, tlsConf *tls.Config) (QpepConnection, error) {
	quicConfig := getConfiguration()

	conn, err := quic.ListenAddr(addr, tlsConf, quicConfig)
	if err != nil {
		logger.Error("Failed to listen on QUIC session: %v\n", err)
		return nil, shared.ErrFailedGatewayConnect
	}

	return &connectionAdapter{
		context:  context.Background(),
		listener: conn,
	}, err
}

func (q *quicGoBackend) Close() error {
	//for _,conn := range q.connections {
	//	_ = conn.Close(code)
	//}
	q.connections = nil
	logger.Info("== QUIC Session Closed ==\n")
	return nil
}

func getConfiguration() *quic.Config {
	return &quic.Config{
		MaxIncomingStreams:      10240,
		DisablePathMTUDiscovery: false,

		HandshakeIdleTimeout: shared.GetScaledTimeout(10, time.Second),
		//KeepAlivePeriod:      1 * time.Second,

		EnableDatagrams: true,
	}
}

type connectionAdapter struct {
	context    context.Context
	listener   quic.Listener
	connection quic.Connection
}

func (c *connectionAdapter) LocalAddr() net.Addr {
	if c.connection != nil {
		return c.connection.LocalAddr()
	}
	if c.listener != nil {
		return c.listener.Addr()
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *connectionAdapter) RemoteAddr() net.Addr {
	if c.connection != nil {
		return c.connection.RemoteAddr()
	}
	if c.listener != nil {
		return c.listener.Addr()
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *connectionAdapter) AcceptStream(ctx context.Context) (QpepStream, error) {
	if c.connection != nil {
		return c.connection.AcceptStream(ctx)
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *connectionAdapter) AcceptConnection(ctx context.Context) (QpepConnection, error) {
	if c.listener != nil {
		conn, err := c.listener.Accept(ctx)
		if err != nil {
			return nil, err
		}
		cNew := &connectionAdapter{
			context:    ctx,
			listener:   c.listener,
			connection: conn,
		}
		return cNew, nil
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *connectionAdapter) Close(code int, message string) error {
	if c.connection != nil {
		return c.connection.CloseWithError(quic.ApplicationErrorCode(code), message)
	}
	if c.listener != nil {
		return c.listener.Close()
	}
	panic(shared.ErrInvalidBackendOperation)
}

var _ QpepConnection = &connectionAdapter{}
