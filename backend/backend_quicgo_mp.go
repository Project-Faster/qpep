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
	"github.com/Project-Faster/qpep/shared/configuration"
	stderr "github.com/Project-Faster/qpep/shared/errors"
	"github.com/Project-Faster/qpep/shared/logger"
	"github.com/Project-Faster/qpep/workers/gateway"
	quic "github.com/project-faster/mp-quic-go"
	"io/ioutil"
	"net"
	"reflect"
	"strings"
	"time"
)

const (
	QUICGO_MP_BACKEND     = "mp-quic-go"
	QUICGO_MP_ALPN        = "qpep_mp"
	QUICGO_MP_DEFAULT_CCA = "reno"
)

var quicGoMpBackendVar QuicBackend = &quicGoMpBackend{}

func init() {
	Register(QUICGO_MP_BACKEND, quicGoMpBackendVar)
}

type quicGoMpBackend struct {
	connections []QuicBackendConnection
}

func (q *quicGoMpBackend) Dial(ctx context.Context, remoteAddress string, port int, clientCertPath string, ccAlgorithm string, ccSlowstartAlgo string, traceOn bool) (QuicBackendConnection, error) {
	quicConfig := quicGoMpGetConfiguration(traceOn)

	var err error
	var session quic.Session

	tlsConf := mpLoadTLSConfig(clientCertPath, "")
	gatewayPath := fmt.Sprintf("%s:%d", remoteAddress, port)

	session, err = quic.DialAddr(gatewayPath, tlsConf, quicConfig)
	if err != nil {
		logger.Error("Unable to Dial Protocol Session: %v\n", err)
		return nil, stderr.ErrFailedGatewayConnect
	}

	sessionAdapter := &quicGoMpConnectionAdapter{
		context:    ctx,
		connection: session,
	}

	q.connections = append(q.connections, sessionAdapter)
	return sessionAdapter, nil
}

func (q *quicGoMpBackend) Listen(ctx context.Context, address string, port int, serverCertPath string, serverKeyPath string, ccAlgorithm string, ccSlowstartAlgo string, traceOn bool) (QuicBackendConnection, error) {
	quicConfig := quicGoMpGetConfiguration(traceOn)

	tlsConf := mpLoadTLSConfig(serverCertPath, serverKeyPath)

	conn, err := quic.ListenAddr(fmt.Sprintf("%s:%d", "0.0.0.0", port), tlsConf, quicConfig)
	if err != nil {
		logger.Error("Failed to listen on Protocol session: %v\n", err)
		return nil, stderr.ErrFailedGatewayConnect
	}

	return &quicGoMpConnectionAdapter{
		context:  ctx,
		listener: conn,
	}, err
}

func (q *quicGoMpBackend) Close() error {
	if !isNil(q.connections) {
		for _, conn := range q.connections {
			_ = conn.Close(0, "")
		}
	}
	q.connections = nil
	logger.Info("== Protocol Session Closed ==\n")
	return nil
}

func quicGoMpGetConfiguration(traceOn bool) *quic.Config {
	multipathAddresses := make([]net.UDPAddr, 0)
	for _, p := range configuration.QPepConfig.Client.MultipathAddressList {
		mAddr := fmt.Sprintf("%s:%d", p.Address, p.Port)
		addr, err := net.ResolveUDPAddr("udp", mAddr)
		logger.OnError(err, "Could not resolve multipath address: "+mAddr)

		if err == nil {
			multipathAddresses = append(multipathAddresses, *addr)
			logger.Info("Added multipath address: %v", mAddr)
		}
	}

	cfg := &quic.Config{
		Versions: []quic.VersionNumber{0},

		MaxReceiveConnectionFlowControlWindow: 10 * 1024 * 1024,
		MaxReceiveStreamFlowControlWindow:     10 * 1024 * 1024,

		IdleTimeout:      2 * time.Second,
		HandshakeTimeout: gateway.GetScaledTimeout(10, time.Second),
		KeepAlive:        false,

		CreatePaths:        true,
		MultipathAddresses: multipathAddresses,
	}

	return cfg
}

type quicGoMpConnectionAdapter struct {
	context    context.Context
	listener   quic.Listener
	connection quic.Session

	streams []quic.Stream
}

func (c *quicGoMpConnectionAdapter) LocalAddr() net.Addr {
	if !isNil(c.connection) {
		return c.connection.LocalAddr()
	}
	if !isNil(c.listener) {
		return c.listener.Addr()
	}
	panic(stderr.ErrInvalidBackendOperation)
}

func (c *quicGoMpConnectionAdapter) RemoteAddr() net.Addr {
	if !isNil(c.connection) {
		return c.connection.RemoteAddr()
	}
	if !isNil(c.listener) {
		return c.listener.Addr()
	}
	panic(stderr.ErrInvalidBackendOperation)
}

func (c *quicGoMpConnectionAdapter) AcceptConnection(ctx context.Context) (QuicBackendConnection, error) {
	if !isNil(c.listener) {
		conn, err := c.listener.Accept()
		if err != nil {
			return nil, err
		}
		cNew := &quicGoMpConnectionAdapter{
			context:    ctx,
			listener:   c.listener,
			connection: conn,
			streams:    make([]quic.Stream, 0, 32),
		}
		return cNew, nil
	}
	panic(stderr.ErrInvalidBackendOperation)
}

func (c *quicGoMpConnectionAdapter) AcceptStream(ctx context.Context) (QuicBackendStream, error) {
	if !isNil(c.connection) {
		stream, err := c.connection.AcceptStream()
		if stream != nil {
			c.streams = append(c.streams, stream)
		}
		return &quicGoMpStreamAdapter{
			Stream: stream,
		}, err
	}
	panic(stderr.ErrInvalidBackendOperation)
}

func (c *quicGoMpConnectionAdapter) OpenStream(ctx context.Context) (QuicBackendStream, error) {
	if !isNil(c.connection) {
		stream, err := c.connection.OpenStreamSync()
		return &quicGoMpStreamAdapter{
			Stream: stream,
		}, err
	}
	panic(stderr.ErrInvalidBackendOperation)
}

func (c *quicGoMpConnectionAdapter) Close(code int, message string) error {
	defer func() {
		c.connection = nil
		c.listener = nil
		c.streams = nil
	}()
	if c.connection != nil {
		err := errors.New(fmt.Sprintf("code:%d,message:%s", code, message))
		for _, st := range c.streams {
			if isNil(st) {
				continue
			}
			st.Reset(err)
			_ = st.Close()
		}
		return c.connection.Close(err)
	}
	if c.listener != nil {
		return c.listener.Close()
	}
	return nil
}

func (c *quicGoMpConnectionAdapter) IsClosed() bool {
	return isNil(c.connection) && isNil(c.listener)
}

var _ QuicBackendConnection = &quicGoMpConnectionAdapter{}

type quicGoMpStreamAdapter struct {
	quic.Stream

	id *uint64

	closedRead  bool
	closedWrite bool
}

func (stream *quicGoMpStreamAdapter) AbortRead(code uint64) {
	err := errors.New(fmt.Sprintf("code:%d", code))
	stream.Reset(err)
	stream.closedRead = true
}

func (stream *quicGoMpStreamAdapter) AbortWrite(code uint64) {
	err := errors.New(fmt.Sprintf("code:%d", code))
	stream.Reset(err)
	stream.closedWrite = true
}

func (stream *quicGoMpStreamAdapter) Sync() bool {
	return stream.IsClosed()
}

func (stream *quicGoMpStreamAdapter) ID() uint64 {
	if !isNil(stream.id) {
		return *stream.id
	}
	stream.id = new(uint64)
	*stream.id = uint64(stream.StreamID())
	return *stream.id
}

func (stream *quicGoMpStreamAdapter) IsClosed() bool {
	return stream.closedRead && stream.closedWrite
}

func (stream *quicGoMpStreamAdapter) Close() error {
	if !stream.IsClosed() {
		return stream.Stream.Close()
	}
	return nil
}

var _ QuicBackendStream = &quicGoMpStreamAdapter{}

// --- Certificate support --- //

func mpLoadTLSConfig(certPEM, keyPEM string) *tls.Config {
	dataCert, err1 := ioutil.ReadFile(certPEM)
	dataKey, err2 := ioutil.ReadFile(keyPEM)

	if err1 != nil {
		logger.Error("Could not find certificate file %s", certPEM)
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
		logger.Error("Certificate file %s does not contain valid certificates", certPEM)
		return nil
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		logger.Error("Certificate parsing in file %s failed: %v", certPEM, err)
		return nil
	}

	if err2 == nil {
		// support not providing private key file

		skippedBlockTypes = skippedBlockTypes[:0]
		var keyDERBlock *pem.Block
		for {
			keyDERBlock, dataKey = pem.Decode(dataKey)
			if keyDERBlock == nil {
				logger.Error("Certificate key parsing in file %s failed", dataKey)
				return nil
			}
			if keyDERBlock.Type == "PRIVATE KEY" || strings.HasSuffix(keyDERBlock.Type, " PRIVATE KEY") {
				break
			}
			skippedBlockTypes = append(skippedBlockTypes, keyDERBlock.Type)
		}

		cert.PrivateKey, err = mpParsePrivateKey(keyDERBlock.Bytes)
		if err != nil {
			logger.Error("Error loading private key from file %s: %v", dataKey, err)
			return nil
		}

		switch pub := x509Cert.PublicKey.(type) {
		case *rsa.PublicKey:
			priv, ok := cert.PrivateKey.(*rsa.PrivateKey)
			if !ok {
				logger.Error("Error loading private key from file %s: Not a valid RSA key", dataKey)
				return nil
			}
			if pub.N.Cmp(priv.N) != 0 {
				logger.Error("Error loading private key from file %s: internal error", dataKey)
				return nil
			}
		case *ecdsa.PublicKey:
			priv, ok := cert.PrivateKey.(*ecdsa.PrivateKey)
			if !ok {
				logger.Error("Error loading private key from file %s: Not a valid ECDSA key", dataKey)
				return nil
			}
			if pub.X.Cmp(priv.X) != 0 || pub.Y.Cmp(priv.Y) != 0 {
				logger.Error("Error loading private key from file %s: internal error", dataKey)
				return nil
			}
		case ed25519.PublicKey:
			priv, ok := cert.PrivateKey.(ed25519.PrivateKey)
			if !ok {
				logger.Error("Error loading private key from file %s: Not a valida ED25519 key", dataKey)
				return nil
			}
			if !bytes.Equal(priv.Public().(ed25519.PublicKey), pub) {
				logger.Error("Error loading private key from file %s: internal error", dataKey)
				return nil
			}
		default:
			logger.Error("Error loading private key from file %s: unsupported key type %v", dataKey, pub)
			return nil
		}
	}

	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		NextProtos:         []string{QUICGO_MP_ALPN},
		InsecureSkipVerify: true,
	}
}

func mpParsePrivateKey(der []byte) (crypto.PrivateKey, error) {
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

// --- isNil --- //
func isNil(i any) bool {
	if i == nil {
		return true
	}
	switch reflect.TypeOf(i).Kind() {
	case reflect.Ptr, reflect.Map, reflect.Array, reflect.Chan, reflect.Slice:
		return reflect.ValueOf(i).IsNil()
	default:
		return false
	}
}
