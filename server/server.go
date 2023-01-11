package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	//"github.com/lucas-clemente/quic-go/logging"
	//"github.com/lucas-clemente/quic-go/qlog"
	"io"
	"io/ioutil"
	"math/big"
	"net"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/client"
	. "github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"

	"github.com/lucas-clemente/quic-go"
)

const (
	INITIAL_BUFF_SIZE = int64(4096)
)

var (
	ServerConfiguration = ServerConfig{
		ListenHost: "0.0.0.0",
		ListenPort: 443,
		APIPort:    444,
	}
	quicListener            quic.Listener
	quicSession             quic.Connection
	QuicServerConfiguration = quic.Config{
		MaxIncomingStreams:      40000,
		DisablePathMTUDiscovery: true,
		//		Tracer: qlog.NewTracer(func(_ logging.Perspective, connID []byte) io.WriteCloser {
		//			filename := fmt.Sprintf("server_%x.qlog", connID)
		//			f, err := os.Create(filename)
		//			if err != nil {
		//				log.Fatal(err)
		//			}
		//			log.Printf("Creating qlog file %s.\n", filename)
		//			return &shared.QLogWriter{Writer: bufio.NewWriter(f)}
		//		}),
	}
)

type ServerConfig struct {
	ListenHost string
	ListenPort int
	APIPort    int
}

func RunServer(ctx context.Context, cancel context.CancelFunc) {
	defer func() {
		if err := recover(); err != nil {
			Info("PANIC: %v\n", err)
			debug.PrintStack()
		}
		if quicListener != nil {
			quicListener.Close()
		}
		cancel()
	}()

	// update configuration from flags
	validateConfiguration()

	listenAddr := ServerConfiguration.ListenHost + ":" + strconv.Itoa(ServerConfiguration.ListenPort)
	Info("Opening QPEP Server on: %s\n", listenAddr)
	var err error
	quicListener, err = quic.ListenAddr(listenAddr, generateTLSConfig(), &client.QuicClientConfiguration)
	if err != nil {
		Info("Encountered error while binding QUIC listener: %s\n", err)
		return
	}

	go ListenQuicSession()

	ctxPerfWatcher, perfWatcherCancel := context.WithCancel(context.Background())
	go performanceWatcher(ctxPerfWatcher)

	for {
		select {
		case <-ctx.Done():
			perfWatcherCancel()
			return
		case <-time.After(10 * time.Millisecond):
			continue
		}
	}
}

func ListenQuicSession() {
	defer func() {
		if err := recover(); err != nil {
			Info("PANIC: %v\n", err)
			debug.PrintStack()
		}
	}()
	for {
		var err error
		quicSession, err = quicListener.Accept(context.Background())
		if err != nil {
			Info("Unrecoverable error while accepting QUIC session: %s\n", err)
			return
		}
		go ListenQuicConn(quicSession)
	}
}

func ListenQuicConn(quicSession quic.Connection) {
	defer func() {
		if err := recover(); err != nil {
			Info("PANIC: %v\n", err)
			debug.PrintStack()
		}
	}()
	for {
		stream, err := quicSession.AcceptStream(context.Background())
		if err != nil {
			if err.Error() != "NO_ERROR: No recent network activity" {
				Info("Unrecoverable error while accepting QUIC stream: %s\n", err)
			}
			return
		}
		Info("Opening QUIC StreamID: %d\n", stream.StreamID())

		go handleQuicStream(stream)
	}
}

func handleQuicStream(stream quic.Stream) {
	defer func() {
		if err := recover(); err != nil {
			Info("PANIC: %v\n", err)
			debug.PrintStack()
		}
	}()

	qpepHeader, err := shared.GetQpepHeader(stream)
	if err != nil {
		Info("Unable to find QPEP header: %s\n", err)
		return
	}

	// To support the server being behind a private NAT (external gateway address != local listening address)
	// we dial the listening address when the connection is directed at the non-local API server
	destAddress := qpepHeader.DestAddr.String()
	if qpepHeader.DestAddr.Port == ServerConfiguration.APIPort {
		destAddress = fmt.Sprintf("%s:%d", ServerConfiguration.ListenHost, ServerConfiguration.APIPort)
	}

	Info(">> Opening TCP Connection to dest:%s, src:%s\n", destAddress, qpepHeader.SourceAddr)
	dial := &net.Dialer{
		LocalAddr: &net.TCPAddr{IP: net.ParseIP(ServerConfiguration.ListenHost)},
		Timeout:   1 * time.Second,
		KeepAlive: 3 * time.Second,
		DualStack: true,
	}
	tcpConn, err := dial.Dial("tcp", destAddress)
	if err != nil {
		Info("Unable to open TCP connection from QPEP stream: %s\n", err)
		return
	}
	Info(">> Opened TCP Conn %s -> %s\n", qpepHeader.SourceAddr, destAddress)

	trackedAddress := qpepHeader.SourceAddr.IP.String()
	proxyAddress := tcpConn.(*net.TCPConn).LocalAddr().String()

	api.Statistics.IncrementCounter(1.0, api.TOTAL_CONNECTIONS)
	api.Statistics.IncrementCounter(1.0, api.PERF_CONN, trackedAddress)
	defer func() {
		api.Statistics.DecrementCounter(1.0, api.PERF_CONN, trackedAddress)
		api.Statistics.DecrementCounter(1.0, api.TOTAL_CONNECTIONS)
	}()

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	var streamWait sync.WaitGroup
	streamWait.Add(2)
	streamQUICtoTCP := func(dst *net.TCPConn, src quic.Stream) {
		defer func() {
			_ = recover()

			api.Statistics.DeleteMappedAddress(proxyAddress)
			streamWait.Done()
		}()

		api.Statistics.SetMappedAddress(proxyAddress, trackedAddress)

		err1 := dst.SetLinger(1)
		if err1 != nil {
			Info("error on setLinger: %s\n", err1)
		}

		var buffSize = INITIAL_BUFF_SIZE
		var loopTimeout = 150 * time.Millisecond
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			src.SetReadDeadline(time.Now().Add(loopTimeout))
			src.SetWriteDeadline(time.Now().Add(loopTimeout))
			dst.SetReadDeadline(time.Now().Add(loopTimeout))
			dst.SetWriteDeadline(time.Now().Add(loopTimeout))

			written, err := io.Copy(dst, src)
			if err != nil || written == 0 {
				if nErr, ok := err.(net.Error); ok && (nErr.Timeout() || nErr.Temporary()) {
					continue
				}
				//log.Printf("Error on Copy %s\n", err)
				break
			}

			api.Statistics.IncrementCounter(float64(written), api.PERF_DW_COUNT, trackedAddress)
			buffSize = int64(written * 2)
			if buffSize < INITIAL_BUFF_SIZE {
				buffSize = INITIAL_BUFF_SIZE
			}
		}
		//log.Printf("Finished Copying Stream ID %d, TCP Conn %s->%s\n", src.StreamID(), dst.LocalAddr().String(), dst.RemoteAddr().String())
	}
	streamTCPtoQUIC := func(dst quic.Stream, src *net.TCPConn) {
		defer func() {
			_ = recover()

			streamWait.Done()
		}()

		err1 := src.SetLinger(1)
		if err1 != nil {
			Info("error on setLinger: %s\n", err1)
		}

		var buffSize = INITIAL_BUFF_SIZE
		var loopTimeout = 150 * time.Millisecond
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			src.SetReadDeadline(time.Now().Add(loopTimeout))
			src.SetWriteDeadline(time.Now().Add(loopTimeout))
			dst.SetReadDeadline(time.Now().Add(loopTimeout))
			dst.SetWriteDeadline(time.Now().Add(loopTimeout))

			written, err := io.Copy(dst, src)
			if err != nil || written == 0 {
				if nErr, ok := err.(net.Error); ok && (nErr.Timeout() || nErr.Temporary()) {
					continue
				}
				//Info("Error on Copy %s\n", err)
				break
			}

			api.Statistics.IncrementCounter(float64(written), api.PERF_UP_COUNT, trackedAddress)
			buffSize = int64(written * 2)
			if buffSize < INITIAL_BUFF_SIZE {
				buffSize = INITIAL_BUFF_SIZE
			}
		}
		//Info("Finished Copying TCP Conn %s->%s, Stream ID %d\n", src.LocalAddr().String(), src.RemoteAddr().String(), dst.StreamID())
	}

	go streamQUICtoTCP(tcpConn.(*net.TCPConn), stream)
	go streamTCPtoQUIC(stream, tcpConn.(*net.TCPConn))

	//we exit (and close the TCP connection) once both streams are done copying or timeout
	streamWait.Wait()
	tcpConn.Close()

	stream.CancelRead(0)
	stream.CancelWrite(0)
	stream.Close()
	Info(">> Closing TCP Conn %s->%s\n", tcpConn.LocalAddr().String(), tcpConn.RemoteAddr().String())
}

func generateTLSConfig() *tls.Config {
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

	ioutil.WriteFile("server_key.pem", keyPEM, 0777)
	ioutil.WriteFile("server_cert.pem", certPEM, 0777)

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"qpep"},
	}
}

func performanceWatcher(ctx context.Context) {
	defer func() {
		if err := recover(); err != nil {
			Info("PANIC: %v\n", err)
			debug.PrintStack()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
			hosts := api.Statistics.GetHosts()

			for _, host := range hosts {
				// load the current count and reset it atomically (so there's no race condition)
				dwCount := api.Statistics.GetCounterAndClear(api.PERF_DW_COUNT, host)
				upCount := api.Statistics.GetCounterAndClear(api.PERF_UP_COUNT, host)
				if dwCount < 0.0 || upCount < 0.0 {
					continue
				}

				// update the speeds
				api.Statistics.SetCounter(dwCount/1024.0, api.PERF_DW_SPEED, host)
				api.Statistics.SetCounter(upCount/1024.0, api.PERF_UP_SPEED, host)

				// update the totals for the client
				api.Statistics.IncrementCounter(dwCount, api.PERF_DW_TOTAL, host)
				api.Statistics.IncrementCounter(upCount, api.PERF_UP_TOTAL, host)
			}
		}
	}
}

func validateConfiguration() {
	ServerConfiguration.ListenHost, _ = shared.GetDefaultLanListeningAddress(shared.QPepConfig.ListenHost, "")
	ServerConfiguration.ListenPort = shared.QPepConfig.ListenPort
	ServerConfiguration.APIPort = shared.QPepConfig.GatewayAPIPort

	shared.AssertParamIP("listen host", ServerConfiguration.ListenHost)
	shared.AssertParamPort("listen port", ServerConfiguration.ListenPort)

	shared.AssertParamPort("api port", ServerConfiguration.APIPort)

	shared.AssertParamPortsDifferent("ports", ServerConfiguration.ListenPort, ServerConfiguration.APIPort)

	Info("Server configuration validation OK\n")
}
