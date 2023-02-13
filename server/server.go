package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/Project-Faster/quic-go"

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
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
)

const (
	// INITIAL_BUFF_SIZE indicates the initial receive buffer for connections
	INITIAL_BUFF_SIZE = int64(4096)
)

var (
	// ServerConfiguration global variable that keeps track of the current server configuration
	ServerConfiguration = ServerConfig{
		ListenHost: "0.0.0.0",
		ListenPort: 443,
		APIPort:    444,
	}
	// quicListener instance of the quic server that receives the connections from clients
	quicListener quic.Listener
)

// ServerConfiguration struct models the parameters necessary for running the quic server
type ServerConfig struct {
	// ListenHost ip address on which the server listens for connections
	ListenHost string
	// ListenPort port [1-65535] on which the server listens for connections
	ListenPort int
	// APIPort port [1-65535] on which the API server is launched
	APIPort int
}

// RunServer method validates the provided server configuration and then launches the server
// with the input context
func RunServer(ctx context.Context, cancel context.CancelFunc) {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v\n", err)
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
	logger.Info("Opening QPEP Server on: %s\n", listenAddr)
	var err error
	quicListener, err = quic.ListenAddr(listenAddr, generateTLSConfig(), shared.GetQuicConfiguration())
	if err != nil {
		logger.Info("Encountered error while binding QUIC listener: %s\n", err)
		return
	}

	// launches listener
	go listenQuicSession()

	ctxPerfWatcher, perfWatcherCancel := context.WithCancel(context.Background())
	go performanceWatcher(ctxPerfWatcher)

	// termination loop
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

// listenQuicSession handles accepting the sessions and the launches goroutines to actually serve them
func listenQuicSession() {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v\n", err)
			debug.PrintStack()
		}
	}()
	for {
		quicSession, err := quicListener.Accept(context.Background())
		if err != nil {
			logger.Error("Unrecoverable error while accepting QUIC session: %s\n", err)
			return
		}
		go listenQuicConn(quicSession)
	}
}

// listenQuicConn handles opened quic sessions and accepts connections in goroutines to actually serve them
func listenQuicConn(quicSession quic.Connection) {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v\n", err)
			debug.PrintStack()
		}
	}()
	for {
		stream, err := quicSession.AcceptStream(context.Background())
		if err != nil {
			if err.Error() != "NO_ERROR: No recent network activity" {
				logger.Error("Unrecoverable error while accepting QUIC stream: %s\n", err)
			}
			return
		}
		logger.Info("Opening QUIC StreamID: %d\n", stream.StreamID())

		go handleQuicStream(stream)
	}
}

// handleQuicStream handles a quic stream connection and bridges to the standard tcp for the common internet
func handleQuicStream(stream quic.Stream) {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v\n", err)
			debug.PrintStack()
		}
	}()

	qpepHeader, err := shared.QPepHeaderFromBytes(stream)
	if err != nil {
		logger.Info("Unable to decode QPEP header: %s\n", err)
		_ = stream.Close()
		return
	}

	// To support the server being behind a private NAT (external gateway address != local listening address)
	// we dial the listening address when the connection is directed at the non-local API server
	destAddress := qpepHeader.DestAddr.String()
	if qpepHeader.DestAddr.Port == ServerConfiguration.APIPort {
		destAddress = fmt.Sprintf("%s:%d", ServerConfiguration.ListenHost, ServerConfiguration.APIPort)
	}

	logger.Info(">> Opening TCP Conn to dest:%s, src:%s\n", destAddress, qpepHeader.SourceAddr)
	dial := &net.Dialer{
		LocalAddr: &net.TCPAddr{IP: net.ParseIP(ServerConfiguration.ListenHost)},
		Timeout:   1 * time.Second,
		KeepAlive: 3 * time.Second,
		DualStack: true,
	}
	tcpConn, err := dial.Dial("tcp", destAddress)
	if err != nil {
		logger.Error("Unable to open TCP connection from QPEP stream: %s\n", err)
		stream.Close()
		return
	}
	logger.Info(">> Opened TCP Conn %s -> %s\n", qpepHeader.SourceAddr, destAddress)

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
			logger.Info("error on setLinger: %s\n", err1)
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
			logger.Info("error on setLinger: %s\n", err1)
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
				//logger.Info("Error on Copy %s\n", err)
				break
			}

			api.Statistics.IncrementCounter(float64(written), api.PERF_UP_COUNT, trackedAddress)
			buffSize = int64(written * 2)
			if buffSize < INITIAL_BUFF_SIZE {
				buffSize = INITIAL_BUFF_SIZE
			}
		}
		//logger.Info("Finished Copying TCP Conn %s->%s, Stream ID %d\n", src.LocalAddr().String(), src.RemoteAddr().String(), dst.StreamID())
	}

	go streamQUICtoTCP(tcpConn.(*net.TCPConn), stream)
	go streamTCPtoQUIC(stream, tcpConn.(*net.TCPConn))

	//we exit (and close the TCP connection) once both streams are done copying or timeout
	streamWait.Wait()
	tcpConn.Close()

	stream.CancelRead(0)
	stream.CancelWrite(0)
	stream.Close()
	logger.Info(">> Closing TCP Conn %s->%s\n", tcpConn.LocalAddr().String(), tcpConn.RemoteAddr().String())
}

// generateTLSConfig creates a new x509 key/certificate pair and dumps it to the disk
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

// performanceWatcher method is a goroutine that checks the current speed of every host every second and
// updates the values for the current speed and total number of bytes uploaded / downloaded
func performanceWatcher(ctx context.Context) {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v\n", err)
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

				// update the speeds and totals for the client
				if dwCount >= 0.0 {
					api.Statistics.SetCounter(dwCount/1024.0, api.PERF_DW_SPEED, host)
					api.Statistics.IncrementCounter(dwCount, api.PERF_DW_TOTAL, host)
				}

				if upCount >= 0.0 {
					api.Statistics.SetCounter(upCount/1024.0, api.PERF_UP_SPEED, host)
					api.Statistics.IncrementCounter(upCount, api.PERF_UP_TOTAL, host)
				}
			}
		}
	}
}

// validateConfiguration method checks the validity of the configuration values that have been provided, panicking if
// there is any issue
func validateConfiguration() {
	shared.AssertParamIP("listen host", shared.QPepConfig.ListenHost)

	ServerConfiguration.ListenHost, _ = shared.GetDefaultLanListeningAddress(shared.QPepConfig.ListenHost, "")
	ServerConfiguration.ListenPort = shared.QPepConfig.ListenPort
	ServerConfiguration.APIPort = shared.QPepConfig.GatewayAPIPort

	shared.AssertParamPort("listen port", ServerConfiguration.ListenPort)

	shared.AssertParamPort("api port", ServerConfiguration.APIPort)

	shared.AssertParamPortsDifferent("ports", ServerConfiguration.ListenPort, ServerConfiguration.APIPort)

	logger.Info("Server configuration validation OK\n")
}
