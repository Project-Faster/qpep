package client

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/parvit/qpep/backend"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/windivert"
	"golang.org/x/net/context"
)

const (
	BUFFER_SIZE = 1024 * 1024

	ACTIVITY_RX_FLAG = "activity_rx"
	ACTIVITY_TX_FLAG = "activity_tx"

	LOCAL_RECONNECTION_RETRIES = 10
)

var (
	// proxyListener listener for the local http connections that get diverted or proxied to the quic server
	proxyListener net.Listener

	newSessionLock sync.RWMutex
	// quicSession listening quic connection to the server
	quicSession backend.QuicBackendConnection
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
		logger.OnError(err, "Unrecoverable error while accepting connection")
		if err != nil {
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
	logger.Info("Accepting TCP connection: source:%s destination:%s", tcpConn.LocalAddr().String(), tcpConn.RemoteAddr().String())
	defer tcpConn.Close()

	tcpSourceAddr := tcpConn.RemoteAddr().(*net.TCPAddr)
	tcpLocalAddr := tcpConn.LocalAddr().(*net.TCPAddr)
	diverted, srcPort, dstPort, srcAddress, dstAddress := windivert.GetConnectionStateData(tcpSourceAddr.Port)

	var proxyRequest *http.Request
	var errProxy error
	if diverted != windivert.DIVERT_OK {
		// proxy open connection
		proxyRequest, errProxy = handleProxyOpenConnection(tcpConn)
		if errProxy == shared.ErrProxyCheckRequest {
			logger.Info("Checked for proxy usage, closing.")
			return
		}
		logger.OnError(errProxy, "opening proxy connection")
	}

	ctx, cancel := context.WithCancel(context.Background())

	var quicStream, err = getQuicStream(ctx)
	if err != nil {
		tcpConn.Close()
		return
	}
	defer quicStream.Close()

	//We want to wait for both the upstream and downstream to finish so we'll set a wait group for the threads
	var streamWait sync.WaitGroup
	streamWait.Add(2)

	//Set our custom header to the QUIC session so the server can generate the correct TCP handshake on the other side
	sessionHeader := shared.QPepHeader{
		SourceAddr: tcpSourceAddr,
		DestAddr:   tcpLocalAddr,
		Flags:      0,
	}

	// divert check
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

		if sessionHeader.DestAddr.IP.String() == ClientConfiguration.GatewayHost {
			sessionHeader.Flags |= shared.QPEP_LOCALSERVER_DESTINATION
		}
		logger.Info("Connection flags : %d %d", sessionHeader.Flags, sessionHeader.Flags&shared.QPEP_LOCALSERVER_DESTINATION)

		logger.Info("Sending QUIC header to server, SourceAddr: %v / DestAddr: %v", sessionHeader.SourceAddr, sessionHeader.DestAddr)
		_, err := quicStream.Write(sessionHeader.ToBytes())
		logger.OnError(err, "writing to quic stream")
	} else {
		if proxyRequest != nil {
			err = handleProxyedRequest(proxyRequest, &sessionHeader, tcpConn, quicStream)
			logger.OnError(err, "handling of proxy proxyRequest")
		}
	}

	//Proxy all stream content from quic to TCP and from TCP to quic
	logger.Info("== Stream %d Start ==", quicStream.ID())
	var activityRX, activityTX = true, true
	ctx = context.WithValue(ctx, ACTIVITY_RX_FLAG, &activityRX)
	ctx = context.WithValue(ctx, ACTIVITY_TX_FLAG, &activityTX)
	defer func() {
		// terminate activity timer
		activityTX = false
		activityRX = false
	}()

	go handleTcpToQuic(ctx, &streamWait, quicStream, tcpConn)
	go handleQuicToTcp(ctx, &streamWait, tcpConn, quicStream)
	go connectionActivityTimer(&activityRX, &activityTX, cancel)

	//we exit (and close the TCP connection) once both streams are done copying
	logger.Info("== Stream %d Wait ==", quicStream.ID())
	streamWait.Wait()
	logger.Info("== Stream %d WaitEnd ==", quicStream.ID())

	quicStream.Close()
	tcpConn.Close()

	logger.Info("== Stream %d End ==", quicStream.ID())

	if !ClientConfiguration.MultiStream {
		// destroy the session so a new one is created next time
		quicSession = nil
	}
}

func connectionActivityTimer(flag_rx, flag_tx *bool, cancelFunc context.CancelFunc) {
	if flag_tx == nil || flag_rx == nil {
		return
	}
	<-time.After(ClientConfiguration.IdleTimeout)
	logger.Debug("activity state: %v / %v", *flag_rx, *flag_tx)
	if !*flag_rx && !*flag_tx {
		cancelFunc()
		return
	}
	go connectionActivityTimer(flag_rx, flag_tx, cancelFunc)
}

// getQuicStream method handles the opening or reutilization of the quic session, and launches a new
// quic stream for communication
func getQuicStream(ctx context.Context) (backend.QuicBackendStream, error) {
	var err error
	var quicStream backend.QuicBackendStream = nil
	var localSession backend.QuicBackendConnection = nil

	newSessionLock.RLock()
	localSession = quicSession
	newSessionLock.RUnlock()

	if localSession == nil {
		newSessionLock.Lock()
		defer newSessionLock.Unlock()

		// open a new quicSession (with all the TLS jazz)
		localSession, err = openQuicSession()
		// if we were unable to open a quic session, drop the TCP connection with RST
		if err != nil {
			return nil, err
		}

		quicSession = localSession
	}

	// if we allow for multiple streams in a session, try and open on the existing session
	if ClientConfiguration.MultiStream && localSession != nil {
		logger.Info("Trying to open on existing session")
		quicStream, err = localSession.OpenStream(context.Background())
		if err == nil {
			logger.Info("Opened a new stream: %d", quicStream.ID())
			return quicStream, nil
		}
		// if we weren't able to open a quicStream on that session (usually inactivity timeout), we can try to open a new session
		logger.OnError(err, "Unable to open new stream on existing QUIC session, closing session")
		quicStream = nil

		newSessionLock.Lock()
		quicSession.Close(0, "Stream could not be opened")
		quicSession = nil
		newSessionLock.Unlock()

		return nil, shared.ErrFailedGatewayConnect
	}

	//Dial a stream to send writtenData on this new session
	quicStream, err = quicSession.OpenStream(ctx)
	// if we cannot open a stream on this session, send a TCP RST and let the client decide to try again
	logger.OnError(err, "Unable to open QUIC stream")
	if err != nil {
		return nil, err
	}
	return quicStream, nil
}

// handleProxyOpenConnection method wraps the logic for intercepting an http request with CONNECT or
// standard method to open the proxy connection correctly via the quic stream
func handleProxyOpenConnection(tcpConn net.Conn) (*http.Request, error) {
	// proxy check
	_ = tcpConn.SetReadDeadline(time.Now().Add(10 * time.Millisecond)) // not scaled as proxy connection is always local

	buf := bytes.NewBuffer(make([]byte, 0, INITIAL_BUFF_SIZE))
	n, err := io.Copy(buf, tcpConn)
	if n == 0 {
		logger.Error("Failed to receive request: %v\n", err)
		return nil, shared.ErrNonProxyableRequest
	}
	if err != nil {
		nErr, ok := err.(net.Error)
		if !ok || (ok && (!nErr.Timeout() && !nErr.Temporary())) {
			_ = tcpConn.Close()
			logger.Error("Failed to receive request: %v\n", err)
			return nil, shared.ErrNonProxyableRequest
		}
	}

	rd := bufio.NewReader(buf)
	req, err := http.ReadRequest(rd)
	if err != nil {
		_ = tcpConn.Close()
		logger.Error("Failed to parse request: %v\n", err)
		return nil, shared.ErrNonProxyableRequest
	}

	if checkProxyTestConnection(req.RequestURI) {
		var isProxyWorking = false
		if shared.UsingProxy && shared.ProxyAddress.String() == "http://"+tcpConn.LocalAddr().String() {
			isProxyWorking = true
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
			Header: http.Header{
				shared.QPEP_PROXY_HEADER: []string{fmt.Sprintf("%v", isProxyWorking)},
			},
		}

		t.Write(tcpConn)
		_ = tcpConn.Close()
		return nil, shared.ErrProxyCheckRequest
	}

	switch req.Method {
	case http.MethodDelete:
		break
	case http.MethodPost:
		break
	case http.MethodPut:
		break
	case http.MethodPatch:
		break
	case http.MethodHead:
		break
	case http.MethodOptions:
		break
	case http.MethodTrace:
		break
	case http.MethodConnect:
		fallthrough
	case http.MethodGet:
		_, _, proxyable := getAddressPortFromHost(req.Host)
		if !proxyable {
			_ = tcpConn.Close()
			logger.Info("Non proxyable request\n")
			return nil, shared.ErrNonProxyableRequest
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
		_ = tcpConn.Close()
		logger.Error("Proxy returns BadGateway\n")
		return nil, shared.ErrNonProxyableRequest
	}
	return req, nil
}

func handleProxyedRequest(req *http.Request, header *shared.QPepHeader, tcpConn net.Conn, stream backend.QuicBackendStream) error {
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
	case http.MethodTrace:
		fallthrough
	case http.MethodGet:
		address, port, proxyable := getAddressPortFromHost(req.Host)
		if !proxyable {
			panic("Should not happen as the handleProxyOpenConnection method checks the http request")
		}

		header.DestAddr = &net.TCPAddr{
			IP:   address,
			Port: port,
		}

		if header.DestAddr.IP.String() == ClientConfiguration.GatewayHost {
			header.Flags |= shared.QPEP_LOCALSERVER_DESTINATION
		}

		logger.Info("Proxied connection flags : %d %d", header.Flags, header.Flags&shared.QPEP_LOCALSERVER_DESTINATION)
		logger.Info("Sending QUIC header to server, SourceAddr: %v / DestAddr: %v", header.SourceAddr, header.DestAddr)

		headerData := header.ToBytes()
		logger.Info("QUIC header: %v", headerData)
		_, err := stream.Write(headerData)
		if err != nil {
			_ = tcpConn.Close()
			logger.Error("Error writing to quic stream: %s", err.Error())
			return shared.ErrFailed
		}

		logger.Info("Sending captured GET request\n")
		err = req.Write(stream)
		if err != nil {
			_ = tcpConn.Close()
			logger.Error("Error writing to tcp stream: %s", err.Error())
			return shared.ErrFailed
		}
		break

	case http.MethodConnect:
		address, port, proxyable := getAddressPortFromHost(req.Host)
		if !proxyable {
			panic("Should not happen as the handleProxyOpenConnection method checks the http request")
		}

		header.DestAddr = &net.TCPAddr{
			IP:   address,
			Port: port,
		}

		if header.DestAddr.IP.String() == ClientConfiguration.GatewayHost {
			header.Flags |= shared.QPEP_LOCALSERVER_DESTINATION
		}
		logger.Info("Proxied connection flags : %d %d", header.Flags, header.Flags&shared.QPEP_LOCALSERVER_DESTINATION)

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

		logger.Info("Proxied connection")
		logger.Info("Sending QUIC header to server, SourceAddr: %v / DestAddr: %v", header.SourceAddr, header.DestAddr)
		_, err := stream.Write(header.ToBytes())
		if err != nil {
			_ = tcpConn.Close()
			logger.Error("Error writing to quic stream: %s", err.Error())
			return shared.ErrFailed
		}
		break
	default:
		panic("Should not happen as the handleProxyOpenConnection method checks the http request method")
	}
	return nil
}

// handleTcpToQuic method implements the tcp connection to quic connection side of the connection
func handleTcpToQuic(ctx context.Context, streamWait *sync.WaitGroup, dst backend.QuicBackendStream, src net.Conn) {
	defer func() {
		_ = recover()

		streamWait.Done()
		logger.Info("== Stream %v TCP->Quic done ==", dst.ID())
	}()

	var activityFlag, ok = ctx.Value(ACTIVITY_RX_FLAG).(*bool)
	if !ok {
		panic("No activity flag set")
	}

	setLinger(src)

	var loopTimeout = shared.GetScaledTimeout(1, time.Second)
	var tempBuffer = make([]byte, BUFFER_SIZE)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var written int = 0
		var err error

		tm := time.Now().Add(loopTimeout)
		_ = src.SetReadDeadline(tm)
		_ = src.SetWriteDeadline(tm)

		read, err_t := src.Read(tempBuffer)

		if err_t != nil {
			*activityFlag = false
			if nErr, ok := err_t.(net.Error); ok && nErr.Timeout() {
				<-time.After(1 * time.Millisecond)
				continue
			}
			_ = dst.Close()
			return
		}
		if read > 0 {
			*activityFlag = true
			_ = dst.SetReadDeadline(tm)
			_ = dst.SetWriteDeadline(tm)

			written, err = dst.Write(tempBuffer[:read])
		}

		if written > 0 {
			*activityFlag = true
			continue
		}

		*activityFlag = false
		if err == nil {
			<-time.After(1 * time.Millisecond)
			continue
		}
		return
	}
	//logger.Info("Finished Copying TCP Conn %s->%s, Stream ID %d\n", src.LocalAddr().String(), src.RemoteAddr().String(), dst.ID())
}

// handleQuicToTcp method implements the quic connection to tcp connection side of the connection
func handleQuicToTcp(ctx context.Context, streamWait *sync.WaitGroup, dst net.Conn, src backend.QuicBackendStream) {
	defer func() {
		_ = recover()

		streamWait.Done()
		logger.Info("== Stream %v Quic->TCP done ==", src.ID())
	}()

	var activityFlag, ok = ctx.Value(ACTIVITY_TX_FLAG).(*bool)
	if !ok {
		panic("No activity flag set")
	}

	setLinger(dst)

	var loopTimeout = shared.GetScaledTimeout(1, time.Second)
	var tempBuffer = make([]byte, BUFFER_SIZE)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var written int = 0
		var err error

		tm := time.Now().Add(loopTimeout)
		_ = src.SetReadDeadline(tm)
		_ = src.SetWriteDeadline(tm)

		read, err_t := src.Read(tempBuffer)

		if err_t != nil {
			*activityFlag = false
			if nErr, ok := err_t.(net.Error); ok && nErr.Timeout() {
				<-time.After(1 * time.Millisecond)
				continue
			}
			_ = dst.Close()
			return
		}
		if read > 0 {
			*activityFlag = true
			_ = dst.SetReadDeadline(tm)
			_ = dst.SetWriteDeadline(tm)

			written, err = dst.Write(tempBuffer[:read])
		}

		if written > 0 {
			*activityFlag = true
			continue
		}

		*activityFlag = false
		if err == nil {
			<-time.After(1 * time.Millisecond)
			continue
		}
		return
	}
}

func setLinger(c net.Conn) {
	if conn, ok := c.(*net.TCPConn); ok {
		err1 := conn.SetLinger(1)
		logger.OnError(err1, "error on setLinger")
	}
}

func checkProxyTestConnection(host string) bool {
	return strings.Contains(host, "qpep-client-proxy-check")
}

// getAddressPortFromHost method returns an address splitted in the corresponding IP, port and if the indicated
// address can be used for proxying
func getAddressPortFromHost(host string) (net.IP, int, bool) {
	var proxyable = false
	var port int64 = 0
	var err error = nil
	var address net.IP
	urlParts := strings.Split(host, ":")
	if len(urlParts) > 2 {
		return nil, 0, false
	}
	if len(urlParts) == 2 {
		port, err = strconv.ParseInt(urlParts[1], 10, 64)
		if err != nil {
			return nil, 0, false
		}
	}

	if urlParts[0] == "" {
		address = net.ParseIP("127.0.0.1")
		proxyable = true
	} else {
		ips, _ := net.LookupIP(urlParts[0])
		for _, ip := range ips {
			address = ip.To4()
			if address == nil {
				continue
			}

			proxyable = true
			break
		}
	}
	return address, int(port), proxyable
}

var quicProvider backend.QuicBackend

// openQuicSession implements the quic connection request to the qpep server
func openQuicSession() (backend.QuicBackendConnection, error) {
	if quicProvider == nil {
		var ok bool
		quicProvider, ok = backend.Get(shared.QPepConfig.Backend)
		if !ok {
			panic(shared.ErrInvalidBackendSelected)
		}
	}

	session, err := quicProvider.Dial(context.Background(), ClientConfiguration.GatewayHost, ClientConfiguration.GatewayPort)

	logger.Info("== Dialing QUIC Session: %s:d ==\n", ClientConfiguration.GatewayHost, ClientConfiguration.GatewayPort)
	if err != nil {
		logger.Error("Unable to Dial QUIC Session: %v\n", err)
		return nil, shared.ErrFailedGatewayConnect
	}
	logger.Info("== QUIC Session Dial: %s:d ==\n", ClientConfiguration.GatewayHost, ClientConfiguration.GatewayPort)

	return session, nil
}