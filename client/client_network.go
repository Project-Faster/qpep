package client

import (
	"bufio"
	"bytes"
	"github.com/parvit/qpep/logger"
	"io/ioutil"
	"net/http"

	"crypto/tls"
	"io"
	"net"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"

	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/windivert"
	"golang.org/x/net/context"
)

var (
	// proxyListener listener for the local http connections that get diverted or proxied to the quic server
	proxyListener net.Listener
	// quicSession listening quic connection to the server
	quicSession quic.Connection
)

// listenTCPConn method implements the routine that listens to incoming diverted/proxied connections
func listenTCPConn(wg *sync.WaitGroup) {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v", err)
			debug.PrintStack()
		}
		wg.Done()
	}()
	for {
		conn, err := proxyListener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				logger.Info("Temporary error when accepting connection: %s", netErr)
			}
			logger.Info("Unrecoverable error while accepting connection: %s", err)
			return
		}

		go handleTCPConn(conn)
	}
}

// handleTCPConn method handles the actual tcp <-> quic connection, using the open session to the server
func handleTCPConn(tcpConn net.Conn) {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v", err)
			debug.PrintStack()
		}
	}()
	logger.Info("Accepting TCP connection from %s with destination of %s", tcpConn.RemoteAddr().String(), tcpConn.LocalAddr().String())
	defer tcpConn.Close()

	var quicStream quic.Stream = nil
	// if we allow for multiple streams in a session, lets try and open on the existing session
	if ClientConfiguration.MultiStream {
		//if we have already opened a quic session, lets check if we've expired our stream
		if quicSession != nil {
			var err error
			logger.Info("Trying to open on existing session")
			quicStream, err = quicSession.OpenStream()
			// if we weren't able to open a quicStream on that session (usually inactivity timeout), we can try to open a new session
			if err != nil {
				logger.Info("Unable to open new stream on existing QUIC session: %s\n", err)
				quicStream = nil
			} else {
				logger.Info("Opened a new stream: %d", quicStream.StreamID())
			}
		}
	}
	// if we haven't opened a stream from multistream, we can open one with a new session
	if quicStream == nil {
		// open a new quicSession (with all the TLS jazz)
		var err error
		quicSession, err = openQuicSession()
		// if we were unable to open a quic session, drop the TCP connection with RST
		if err != nil {
			return
		}

		//Open a stream to send data on this new session
		quicStream, err = quicSession.OpenStreamSync(context.Background())
		// if we cannot open a stream on this session, send a TCP RST and let the client decide to try again
		if err != nil {
			logger.Info("Unable to open QUIC stream: %s\n", err)
			return
		}
	}
	defer quicStream.Close()

	//We want to wait for both the upstream and downstream to finish so we'll set a wait group for the threads
	var streamWait sync.WaitGroup
	streamWait.Add(2)

	//Set our custom header to the QUIC session so the server can generate the correct TCP handshake on the other side
	sessionHeader := shared.QPepHeader{
		SourceAddr: tcpConn.RemoteAddr().(*net.TCPAddr),
		DestAddr:   tcpConn.LocalAddr().(*net.TCPAddr),
	}

	// divert check
	diverted, srcPort, dstPort, srcAddress, dstAddress := windivert.GetConnectionStateData(sessionHeader.SourceAddr.Port)
	if diverted == windivert.DIVERT_OK {
		logger.Info("Diverted connection: %v:%v %v:%v", srcAddress, srcPort, dstAddress, dstPort)

		sessionHeader.SourceAddr = &net.TCPAddr{
			IP:   net.ParseIP(srcAddress),
			Port: srcPort,
		}
		sessionHeader.DestAddr = &net.TCPAddr{
			IP:   net.ParseIP(dstAddress),
			Port: dstPort,
		}

		logger.Info("Sending QUIC header to server, SourceAddr: %v / DestAddr: %v", sessionHeader.SourceAddr, sessionHeader.DestAddr)
		_, err := quicStream.Write(sessionHeader.ToBytes())
		if err != nil {
			logger.Info("Error writing to quic stream: %s", err.Error())
		}
	} else {
		_ = handleProxyOpenConnection(&sessionHeader, tcpConn, quicStream)
	}

	ctx, _ := context.WithTimeout(context.Background(), ClientConfiguration.IdleTimeout)

	//Proxy all stream content from quic to TCP and from TCP to quic
	logger.Info("== Stream %d Start ==", quicStream.StreamID())
	go tcpToQuicHandler(ctx, &streamWait, quicStream, tcpConn.(*net.TCPConn))
	go quicToTCPHandler(ctx, &streamWait, tcpConn.(*net.TCPConn), quicStream)

	//we exit (and close the TCP connection) once both streams are done copying
	streamWait.Wait()
	tcpConn.(*net.TCPConn).SetLinger(3)
	tcpConn.Close()

	quicStream.CancelWrite(0)
	quicStream.CancelRead(0)
	quicStream.Close()
	logger.Info("== Stream %d End ==", quicStream.StreamID())
}

func handleProxyOpenConnection(header *shared.QPepHeader, tcpConn net.Conn, stream quic.Stream) error {
	// proxy check
	tcpConn.SetReadDeadline(time.Now().Add(1 * time.Second))

	buf := bytes.NewBuffer([]byte{})
	io.Copy(buf, tcpConn)

	rd := bufio.NewReader(buf)
	req, err := http.ReadRequest(rd)
	if err != nil {
		tcpConn.Close()
		logger.Info("Failed to parse request: %v\n", err)
		return shared.ErrNonProxyableRequest
	}

	switch req.Method {
	case http.MethodDelete:
		fallthrough
	case http.MethodPost:
		fallthrough
	case http.MethodPut:
		fallthrough
	case http.MethodPatch:
		fallthrough
	case http.MethodHead:
		fallthrough
	case http.MethodOptions:
		fallthrough
	case http.MethodGet:
		address, port, proxyable := getAddressPortFromHost(req.Host)
		if !proxyable {
			tcpConn.Close()
			logger.Info("Non proxyable request\n")
			return shared.ErrNonProxyableRequest
		}

		header.DestAddr = &net.TCPAddr{
			IP:   address,
			Port: port,
		}

		logger.Info("Proxied connection")
		logger.Info("Sending QUIC header to server, SourceAddr: %v / DestAddr: %v", header.SourceAddr, header.DestAddr)
		_, err := stream.Write(header.ToBytes())
		if err != nil {
			logger.Error("Error writing to quic stream: %s", err.Error())
			return shared.ErrFailed
		}

		logger.Info("Sending captured GET request\n")
		err = req.Write(stream)
		if err != nil {
			logger.Error("Error writing to tcp stream: %s", err.Error())
			return shared.ErrFailed
		}
		break

	case http.MethodConnect:
		address, port, proxyable := getAddressPortFromHost(req.Host)
		if !proxyable {
			tcpConn.Close()
			logger.Error("Non proxyable request\n")
			return shared.ErrNonProxyableRequest
		}

		header.DestAddr = &net.TCPAddr{
			IP:   address,
			Port: port,
		}

		t := http.Response{
			Status:        http.StatusText(http.StatusOK),
			StatusCode:    http.StatusOK,
			Proto:         req.Proto,
			ProtoMajor:    req.ProtoMajor,
			ProtoMinor:    req.ProtoMinor,
			Body:          ioutil.NopCloser(bytes.NewBufferString("")),
			ContentLength: 0,
			Request:       req,
			Header:        make(http.Header, 0),
		}

		t.Write(tcpConn)
		buf.Reset()

		logger.Info("Proxied connection")
		logger.Info("Sending QUIC header to server, SourceAddr: %v / DestAddr: %v", header.SourceAddr, header.DestAddr)
		_, err := stream.Write(header.ToBytes())
		if err != nil {
			logger.Error("Error writing to quic stream: %s", err.Error())
			return shared.ErrFailed
		}
		break
	default:
		t := http.Response{
			Status:        http.StatusText(http.StatusBadGateway),
			StatusCode:    http.StatusBadGateway,
			Proto:         req.Proto,
			ProtoMajor:    req.ProtoMajor,
			ProtoMinor:    req.ProtoMinor,
			Body:          ioutil.NopCloser(bytes.NewBufferString("")),
			ContentLength: 0,
			Request:       req,
			Header:        make(http.Header, 0),
		}

		t.Write(tcpConn)
		tcpConn.Close()
		logger.Error("Proxy returns BadGateway\n")
		return shared.ErrFailed
	}
	return nil
}

func tcpToQuicHandler(ctx context.Context, streamWait *sync.WaitGroup, dst quic.Stream, src *net.TCPConn) {
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

		written, err := io.Copy(dst, io.LimitReader(src, buffSize))
		if err != nil || written == 0 {
			if nErr, ok := err.(net.Error); ok && (nErr.Timeout() || nErr.Temporary()) {
				continue
			}
			//logger.Info("Error on Copy %s\n", err)
			break
		}

		buffSize = int64(written * 2)
		if buffSize < INITIAL_BUFF_SIZE {
			buffSize = INITIAL_BUFF_SIZE
		}
	}
	//logger.Info("Finished Copying TCP Conn %s->%s, Stream ID %d\n", src.LocalAddr().String(), src.RemoteAddr().String(), dst.StreamID())
}

func quicToTCPHandler(ctx context.Context, streamWait *sync.WaitGroup, dst *net.TCPConn, src quic.Stream) {
	defer func() {
		_ = recover()
		streamWait.Done()
	}()

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

		written, err := io.Copy(dst, io.LimitReader(src, buffSize))
		if err != nil || written == 0 {
			if nErr, ok := err.(net.Error); ok && (nErr.Timeout() || nErr.Temporary()) {
				continue
			}
			//logger.Info("Error on Copy %s\n", err)
			break
		}

		buffSize = int64(written * 2)
		if buffSize < INITIAL_BUFF_SIZE {
			buffSize = INITIAL_BUFF_SIZE
		}
	}
	//logger.Info("Finished Copying Stream ID %d, TCP Conn %s->%s\n", src.StreamID(), dst.LocalAddr().String(), dst.RemoteAddr().String())
}

// getAddressPortFromHost method returns an address splitted in the corresponding IP, port and if the indicated
// address can be used for proxying
func getAddressPortFromHost(host string) (net.IP, int, bool) {
	var proxyable = false
	var port int64 = 0
	var address net.IP
	urlParts := strings.Split(host, ":")
	if len(urlParts) > 2 {
		return nil, 0, false
	}
	if len(urlParts) == 2 {
		port, _ = strconv.ParseInt(urlParts[1], 10, 64)
	}

	ips, _ := net.LookupIP(urlParts[0])
	for _, ip := range ips {
		address = ip.To4()
		if address == nil {
			continue
		}

		proxyable = true
		break
	}
	return address, int(port), proxyable
}

// openQuicSession implements the quic connection request to the qpep server
func openQuicSession() (quic.Connection, error) {
	var err error
	var session quic.Connection
	tlsConf := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"qpep"}}
	gatewayPath := ClientConfiguration.GatewayHost + ":" + strconv.Itoa(ClientConfiguration.GatewayPort)
	quicClientConfig := shared.GetQuicConfiguration()

	logger.Info("Dialing QUIC Session: %s\n", gatewayPath)
	for i := 0; i < ClientConfiguration.MaxConnectionRetries; i++ {
		session, err = quic.DialAddr(gatewayPath, tlsConf, quicClientConfig)
		if err == nil {
			logger.Info("QUIC Session Open\n")
			return session, nil
		}
		logger.Info("Failed to Open QUIC Session: %s\n    Retrying...\n", err)
	}

	logger.Error("Max Retries Exceeded. Unable to Open QUIC Session: %s\n", err)
	return nil, shared.ErrFailedGatewayConnect
}
