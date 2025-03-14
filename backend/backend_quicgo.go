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
	stderr "github.com/Project-Faster/qpep/shared/errors"
	"github.com/Project-Faster/qpep/shared/logger"
	"github.com/Project-Faster/qpep/workers/gateway"
	"github.com/Project-Faster/quic-go"
	"github.com/Project-Faster/quic-go/logging"
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

var quicGoBackendVar QuicBackend = &quicGoBackend{}

func init() {
	Register(QUICGO_BACKEND, quicGoBackendVar)
}

type quicGoBackend struct {
	connections []QuicBackendConnection
}

func (q *quicGoBackend) Dial(ctx context.Context, remoteAddress string, port int, clientCertPath string, ccAlgorithm string, ccSlowstartAlgo string, traceOn bool) (QuicBackendConnection, error) {
	quicConfig := quicGoGetConfiguration(traceOn)

	var err error
	var session quic.Connection

	tlsConf := loadTLSConfig(clientCertPath, "")
	gatewayPath := fmt.Sprintf("%s:%d", remoteAddress, port)

	session, err = quic.DialAddr(gatewayPath, tlsConf, quicConfig)
	if err != nil {
		logger.Error("Unable to Dial Protocol Session: %v\n", err)
		return nil, stderr.ErrFailedGatewayConnect
	}

	sessionAdapter := &quicGoConnectionAdapter{
		context:    ctx,
		connection: session,
	}

	q.connections = append(q.connections, sessionAdapter)
	return sessionAdapter, nil
}

func (q *quicGoBackend) Listen(ctx context.Context, address string, port int, serverCertPath string, serverKeyPath string, ccAlgorithm string, ccSlowstartAlgo string, traceOn bool) (QuicBackendConnection, error) {
	quicConfig := quicGoGetConfiguration(traceOn)

	tlsConf := loadTLSConfig(serverCertPath, serverKeyPath)

	conn, err := quic.ListenAddr(fmt.Sprintf("%s:%d", address, port), tlsConf, quicConfig)
	if err != nil {
		logger.Error("Failed to listen on Protocol session: %v\n", err)
		return nil, stderr.ErrFailedGatewayConnect
	}

	return &quicGoConnectionAdapter{
		context:  ctx,
		listener: conn,
	}, err
}

func (q *quicGoBackend) Close() error {
	for _, conn := range q.connections {
		_ = conn.Close(0, "")
	}
	q.connections = nil
	logger.Info("== Protocol Session Closed ==\n")
	return nil
}

func quicGoGetConfiguration(traceOn bool) *quic.Config {
	cfg := &quic.Config{
		MaxIncomingStreams:      1024,
		DisablePathMTUDiscovery: false,
		MaxIdleTimeout:          2 * time.Second,

		InitialConnectionReceiveWindow: 10 * 1024 * 1024,

		HandshakeIdleTimeout: gateway.GetScaledTimeout(10, time.Second),
		KeepAlivePeriod:      0,

		EnableDatagrams: false,
	}
	if traceOn {
		cfg.Tracer = &qpepQuicTracer{}
	}

	return cfg
}

type quicGoConnectionAdapter struct {
	context    context.Context
	listener   quic.Listener
	connection quic.Connection

	streams []quic.Stream
}

func (c *quicGoConnectionAdapter) LocalAddr() net.Addr {
	if c.connection != nil {
		return c.connection.LocalAddr()
	}
	if c.listener != nil {
		return c.listener.Addr()
	}
	panic(stderr.ErrInvalidBackendOperation)
}

func (c *quicGoConnectionAdapter) RemoteAddr() net.Addr {
	if c.connection != nil {
		return c.connection.RemoteAddr()
	}
	if c.listener != nil {
		return c.listener.Addr()
	}
	panic(stderr.ErrInvalidBackendOperation)
}

func (c *quicGoConnectionAdapter) AcceptConnection(ctx context.Context) (QuicBackendConnection, error) {
	if c.listener != nil {
		conn, err := c.listener.Accept(ctx)
		if err != nil {
			return nil, err
		}
		cNew := &quicGoConnectionAdapter{
			context:    ctx,
			listener:   c.listener,
			connection: conn,
			streams:    make([]quic.Stream, 0, 32),
		}
		return cNew, nil
	}
	panic(stderr.ErrInvalidBackendOperation)
}

func (c *quicGoConnectionAdapter) AcceptStream(ctx context.Context) (QuicBackendStream, error) {
	if c.connection != nil {
		stream, err := c.connection.AcceptStream(ctx)
		if stream != nil {
			c.streams = append(c.streams, stream)
		}
		return &quicGoStreamAdapter{
			Stream: stream,
		}, err
	}
	panic(stderr.ErrInvalidBackendOperation)
}

func (c *quicGoConnectionAdapter) OpenStream(ctx context.Context) (QuicBackendStream, error) {
	if c.connection != nil {
		stream, err := c.connection.OpenStreamSync(ctx)
		return &quicGoStreamAdapter{
			Stream: stream,
		}, err
	}
	panic(stderr.ErrInvalidBackendOperation)
}

func (c *quicGoConnectionAdapter) Close(code int, message string) error {
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

func (c *quicGoConnectionAdapter) IsClosed() bool {
	return c.connection == nil && c.listener == nil
}

var _ QuicBackendConnection = &quicGoConnectionAdapter{}

type quicGoStreamAdapter struct {
	quic.Stream

	id *uint64

	closedRead  bool
	closedWrite bool
}

func (stream *quicGoStreamAdapter) AbortRead(code uint64) {
	stream.CancelRead(quic.StreamErrorCode(code))
	stream.closedRead = true
}

func (stream *quicGoStreamAdapter) AbortWrite(code uint64) {
	stream.CancelWrite(quic.StreamErrorCode(code))
	stream.closedWrite = true
}

func (stream *quicGoStreamAdapter) Sync() bool {
	return stream.IsClosed()
}

func (stream *quicGoStreamAdapter) ID() uint64 {
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

func (stream *quicGoStreamAdapter) IsClosed() bool {
	return false // stream.closedRead || stream.closedWrite
}

func (stream *quicGoStreamAdapter) Close() error {
	ctx := stream.Stream.Context()
	<-ctx.Done()

	return stream.Stream.Close()
}

var _ QuicBackendStream = &quicGoStreamAdapter{}

// --- Certificate support --- //

func loadTLSConfig(certPEM, keyPEM string) *tls.Config {
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

		cert.PrivateKey, err = parsePrivateKey(keyDERBlock.Bytes)
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

// --- Tracer --- //
type qpepQuicTracer struct {
	logging.NullTracer
}

var tracer = &qpepQuicConnectionTracer{}

func (t *qpepQuicTracer) TracerForConnection(ctx context.Context, p logging.Perspective, odcid logging.ConnectionID) logging.ConnectionTracer {
	return tracer
}
func (t *qpepQuicTracer) SentPacket(addr net.Addr, hdr *logging.Header, count logging.ByteCount, frames []logging.Frame) {
	logger.Info("[QGO] Sent packet to %s: %s %d", addr, hdr.PacketType(), count)
}
func (t *qpepQuicTracer) DroppedPacket(addr net.Addr, typePkt logging.PacketType, count logging.ByteCount, reason logging.PacketDropReason) {
	logger.Info("[QGO] Dropped packet to %s: %d %d - %v", addr, typePkt, count, reason)
}

type qpepQuicConnectionTracer struct {
	logging.NullConnectionTracer
}

func (n *qpepQuicConnectionTracer) SentLongHeaderPacket(hdr *logging.ExtendedHeader, count logging.ByteCount, _ *logging.AckFrame, frames []logging.Frame) {
	logger.Info("[QGO] Sent packet (long) %v: %d", hdr, count)
}
func (n *qpepQuicConnectionTracer) SentShortHeaderPacket(hdr *logging.ShortHeader, count logging.ByteCount, _ *logging.AckFrame, frames []logging.Frame) {
	logger.Info("[QGO] Sent packet (short) %v: %d", hdr, count)
}
func (n *qpepQuicConnectionTracer) ReceivedRetry(hdr *logging.Header) {
	logger.Info("[QGO] Retry packet %v", hdr)
}
func (n *qpepQuicConnectionTracer) ReceivedLongHeaderPacket(hdr *logging.ExtendedHeader, count logging.ByteCount, frames []logging.Frame) {
	logger.Info("[QGO] Recv packet (long) %v: %d", hdr, count)
}
func (n *qpepQuicConnectionTracer) ReceivedShortHeaderPacket(hdr *logging.ShortHeader, count logging.ByteCount, frames []logging.Frame) {
	logger.Info("[QGO] Recv packet (short) %v: %d", hdr, count)
}

func (n *qpepQuicConnectionTracer) BufferedPacket(typePkt logging.PacketType, count logging.ByteCount) {
	logger.Info("[QGO] Buffered packet %d: %d", typePkt, count)
}
func (n *qpepQuicConnectionTracer) AcknowledgedPacket(level logging.EncryptionLevel, number logging.PacketNumber) {
	logger.Info("[QGO] Ack packet %d", number)
}
func (n *qpepQuicConnectionTracer) LostPacket(level logging.EncryptionLevel, number logging.PacketNumber, reason logging.PacketLossReason) {
	logger.Info("[QGO] Lost packet %d - %v", number, reason)
}
func (n *qpepQuicConnectionTracer) UpdatedCongestionState(state logging.CongestionState) {
	logger.Info("[QGO] congestion changed to state %v", state)
}
func (n *qpepQuicConnectionTracer) Close() {
	logger.Info("[QGO] Close")
}

func (n *qpepQuicConnectionTracer) ClosedConnection(err error) {
	logger.Info("[QGO] Close Connection: %v", err)
}
