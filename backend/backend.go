package backend

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"io/ioutil"
	"math/big"
	"net"
	"time"
)

type QuicBackend interface {
	Dial(ctx context.Context, remoteAddress string, port int) (QuicBackendConnection, error)
	Listen(ctx context.Context, address string, port int) (QuicBackendConnection, error)
	Close() error
}

type QuicBackendConnection interface {
	// LocalAddr returns the local address.
	LocalAddr() net.Addr
	// RemoteAddr returns the address of the peer.
	RemoteAddr() net.Addr
	OpenStream(context.Context) (QuicBackendStream, error)
	AcceptStream(context.Context) (QuicBackendStream, error)
	AcceptConnection(context.Context) (QuicBackendConnection, error)
	Close(code int, message string) error
}

type QuicBackendStream interface {
	io.Reader
	io.Writer
	io.Closer

	ID() uint64

	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

// generateTLSConfig creates a new x509 key/certificate pair and dumps it to the disk
func generateTLSConfig(fileprefix string) *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	ioutil.WriteFile(fileprefix+"_key.pem", keyPEM, 0777)
	ioutil.WriteFile(fileprefix+"_cert.pem", certPEM, 0777)

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"qpep"},
	}
}
