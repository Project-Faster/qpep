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
	"github.com/Project-Faster/quic-go"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"io/ioutil"
	"net"
	"strings"
	"time"
)

const (
	QUICGO_BACKEND     = "quic-go"
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

func (q *quicGoBackend) Dial(ctx context.Context, remoteAddress string, port int, clientCertPath string, ccAlgorithm string, traceOn bool) (QuicBackendConnection, error) {
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

func (q *quicGoBackend) Listen(ctx context.Context, address string, port int, serverCertPath, serverKeyPath, ccAlgorithm string, traceOn bool) (QuicBackendConnection, error) {
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

		HandshakeIdleTimeout: shared.GetScaledTimeout(10, time.Second),
		//KeepAlivePeriod:      1 * time.Second,

		EnableDatagrams: false,
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
	defer func() {
		c.connection = nil
		c.listener = nil
	}()
	if c.connection != nil {
		return c.connection.CloseWithError(quic.ApplicationErrorCode(code), message)
	}
	if c.listener != nil {
		return c.listener.Close()
	}
	panic(shared.ErrInvalidBackendOperation)
}

func (c *qgoConnectionAdapter) IsClosed() bool {
	return c.connection == nil && c.listener == nil
}

var _ QuicBackendConnection = &qgoConnectionAdapter{}

type qgoStreamAdapter struct {
	quic.Stream

	id *uint64
}

func (stream *qgoStreamAdapter) AbortRead(code uint64) {
	stream.CancelRead(quic.StreamErrorCode(code))
}

func (stream *qgoStreamAdapter) AbortWrite(code uint64) {
	stream.CancelWrite(quic.StreamErrorCode(code))
}

func (stream *qgoStreamAdapter) Sync() bool {
	return true
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
	return false
}

var _ QuicBackendStream = &qgoStreamAdapter{}

// --- Certificate support --- //

func loadTLSConfig(certPEM, keyPEM string) *tls.Config {
	dataCert, err1 := ioutil.ReadFile(certPEM)
	dataKey, err2 := ioutil.ReadFile(keyPEM)

	if err1 != nil {
		return nil
	}

	var cert tls.Certificate
	var skippedBlockTypes []string
	for {
		var certDERBlock *pem.Block
		certDERBlock, dataCert = pem.Decode(dataCert)
		if certDERBlock == nil {
			break
		}
		if certDERBlock.Type == "CERTIFICATE" {
			cert.Certificate = append(cert.Certificate, certDERBlock.Bytes)
		} else {
			skippedBlockTypes = append(skippedBlockTypes, certDERBlock.Type)
		}
	}

	if len(cert.Certificate) == 0 {
		return nil
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil
	}

	if err2 == nil {
		// support not providing private key file

		skippedBlockTypes = skippedBlockTypes[:0]
		var keyDERBlock *pem.Block
		for {
			keyDERBlock, dataKey = pem.Decode(dataKey)
			if keyDERBlock == nil {
				return nil
			}
			if keyDERBlock.Type == "PRIVATE KEY" || strings.HasSuffix(keyDERBlock.Type, " PRIVATE KEY") {
				break
			}
			skippedBlockTypes = append(skippedBlockTypes, keyDERBlock.Type)
		}

		cert.PrivateKey, err = parsePrivateKey(keyDERBlock.Bytes)
		if err != nil {
			return nil
		}

		switch pub := x509Cert.PublicKey.(type) {
		case *rsa.PublicKey:
			priv, ok := cert.PrivateKey.(*rsa.PrivateKey)
			if !ok {
				return nil
			}
			if pub.N.Cmp(priv.N) != 0 {
				return nil
			}
		case *ecdsa.PublicKey:
			priv, ok := cert.PrivateKey.(*ecdsa.PrivateKey)
			if !ok {
				return nil
			}
			if pub.X.Cmp(priv.X) != 0 || pub.Y.Cmp(priv.Y) != 0 {
				return nil
			}
		case ed25519.PublicKey:
			priv, ok := cert.PrivateKey.(ed25519.PrivateKey)
			if !ok {
				return nil
			}
			if !bytes.Equal(priv.Public().(ed25519.PublicKey), pub) {
				return nil
			}
		default:
			return nil
		}
	}

	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		NextProtos:         []string{QUICGO_ALPN},
		InsecureSkipVerify: true,
	}
}

func parsePrivateKey(der []byte) (crypto.PrivateKey, error) {
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		switch key := key.(type) {
		case *rsa.PrivateKey, *ecdsa.PrivateKey, ed25519.PrivateKey:
			return key, nil
		default:
			return nil, errors.New("tls: found unknown private key type in PKCS#8 wrapping")
		}
	}
	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return key, nil
	}

	return nil, errors.New("tls: failed to parse private key")
}
