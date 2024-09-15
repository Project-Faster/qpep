package backend

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/Project-Faster/qpep/shared"
	"github.com/Project-Faster/qpep/shared/logger"
	"hash/crc64"
	"io"
	"io/ioutil"
	"math/big"
	"net"
	"os"
	"time"
)

var (
	localPacketCounter = 0
	crcTable           = crc64.MakeTable(crc64.ISO)
)

type QuicBackend interface {
	Dial(ctx context.Context, remoteAddress string, port int, clientCertPath string, ccAlgorithm string, ccSlowstartAlgo string, traceOn bool) (QuicBackendConnection, error)
	Listen(ctx context.Context, address string, port int, serverCertPath string, serverKeyPath string, ccAlgorithm string, ccSlowstartAlgo string, traceOn bool) (QuicBackendConnection, error)
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

	IsClosed() bool
}

type QuicBackendStream interface {
	io.Reader
	io.Writer
	io.Closer

	ID() uint64

	Sync() bool
	AbortRead(code uint64)
	AbortWrite(code uint64)

	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error

	IsClosed() bool
}

type ReaderTimeout interface {
	io.Reader
	SetReadDeadline(time.Time) error
}

type WriterTimeout interface {
	io.Writer
	SetWriteDeadline(time.Time) error
}

func CopyBuffer(dst WriterTimeout, src ReaderTimeout, buf []byte, timeout time.Duration, debugPrefix string) (written, read int64, err error) {
	src.SetReadDeadline(time.Now().Add(timeout))

	var nr int
	var er error
	var nw int
	var ew error

	nr, er = src.Read(buf)

	if nr > 0 {
		read += int64(nr)

		dumpPacket("rd", debugPrefix, buf, nr)

		offset := 0
		for err == nil && written != read {
			dst.SetWriteDeadline(time.Now().Add(timeout))
			nw, ew = dst.Write(buf[offset:nr])
			written += int64(nw)
			offset += nw
			if ew == nil && nw <= 0 {
				nw = 0
				ew = io.ErrUnexpectedEOF
				err = io.ErrUnexpectedEOF
			}
			if ew != nil {
				if err2, ok := ew.(net.Error); ok && err2.Timeout() {
					continue
				}
				err = ew
			}
		}

		dumpPacket("wr", debugPrefix, buf, nw)

	} else {
		if er != nil {
			err = er
		}
	}
	if er == io.EOF || ew == io.EOF {
		err = nil
	}

	logger.Debug("[%d][%s] %d,%v wr: %d,%v - %v **", localPacketCounter, debugPrefix, nr, er, nw, ew, err)

	localPacketCounter++
	return written, read, err
}

func dumpPacket(dmpType, prefix string, buf []byte, nr int) {
	if !shared.DEBUG_DUMP_PACKETS {
		return
	}

	dump, derr := os.Create(fmt.Sprintf("%s.%d-%s.bin", prefix, localPacketCounter, dmpType))
	if derr != nil {
		panic(derr)
	}
	dump.Write(buf[0:nr])
	dump.Sync()
	dump.Close()
	logger.Debug("[%d][%s] %s: %d (%v)", localPacketCounter, dump.Name(), dmpType, nr, crc64.Checksum(buf[0:nr], crcTable))
}

// GenerateTLSConfig creates a new x509 key/certificate pair and dumps it to the disk
func GenerateTLSConfig(certfile, keyfile string) tls.Certificate {
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

	ioutil.WriteFile(certfile, certPEM, 0777)
	ioutil.WriteFile(keyfile, keyPEM, 0777)

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return tlsCert
}
