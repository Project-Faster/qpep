package client

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/parvit/qpep/shared/configuration"
	"github.com/parvit/qpep/shared/errors"
	"github.com/parvit/qpep/shared/flags"
	"github.com/parvit/qpep/shared/logger"
	"github.com/parvit/qpep/shared/protocol"
	"github.com/parvit/qpep/workers/gateway"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/parvit/qpep/backend"
	"github.com/parvit/qpep/shared"
	"golang.org/x/net/context"
)

var (
	// proxyListener listener for the local http connections that get diverted or proxied to the quic server
	proxyListener net.Listener

	newSessionLock sync.RWMutex
	// quicSession listening quic connection to the server
	quicSession backend.QuicBackendConnection

	filteredPorts map[int]struct{} = nil
)

func setLinger(c net.Conn) {
	if conn, ok := c.(*net.TCPConn); ok {
		err1 := conn.SetLinger(1)
		logger.OnError(err1, "error on setLinger")
	}
}

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
	startTime := time.Now()

	setLinger(tcpConn)

	logger.Info("TCP connection START: source:%s destination:%s", tcpConn.LocalAddr().String(), tcpConn.RemoteAddr().String())
	defer func() {
		logger.Debug("TCP connection END: source:%s destination:%s", tcpConn.LocalAddr().String(), tcpConn.RemoteAddr().String())
		_ = tcpConn.Close()
	}()

	tcpRemoteAddr := tcpConn.RemoteAddr().(*net.TCPAddr)
	tcpLocalAddr := tcpConn.LocalAddr().(*net.TCPAddr)
	diverted, srcPort, dstPort, srcAddress, dstAddress := gateway.GetConnectionDivertedState(tcpLocalAddr, tcpRemoteAddr)

	var proxyRequest *http.Request
	var errProxy error
	if !diverted {
		if filteredPorts == nil {
			filteredPorts = make(map[int]struct{})
			for _, p := range configuration.QPepConfig.Limits.IgnoredPorts {
				filteredPorts[p] = struct{}{}
			}
		}

		if filteredPorts == nil {
			filteredPorts = make(map[int]struct{})
			for _, p := range shared.QPepConfig.IgnoredPorts {
				filteredPorts[p] = struct{}{}
			}
		}

		// proxy open connection
		proxyRequest, errProxy = handleProxyOpenConnection(tcpConn)
		if errProxy == errors.ErrProxyCheckRequest {
			logger.Info("Checked for proxy usage, closing.")
			return
		}

		// check direct connection
		if proxyRequest != nil {
			_, port, _ := getAddressPortFromHost(proxyRequest.Host)
			if _, ok := filteredPorts[port]; ok {
				logger.Info("opening proxy direct connection")
				handleProxyedRequest(proxyRequest, nil, tcpConn, nil)
				return
			}
		}

		logger.OnError(errProxy, "opening proxy connection")
	}

	ctx, _ := context.WithCancel(context.Background())

	var quicStream, err = getQuicStream(ctx)
	if err != nil {
		tcpConn.Close()
		return
	}
	defer quicStream.Close()

	//We want to wait for both the upstream and downstream to finish so we'll set a wait group for the threads
	var streamWait sync.WaitGroup
	streamWait.Add(2)

	//Set our custom header to the Protocol session so the server can generate the correct TCP handshake on the other side
	sessionHeader := protocol.QPepHeader{
		SourceAddr: tcpRemoteAddr,
		DestAddr:   tcpLocalAddr,
		Flags:      0,
	}

	generalConfig := configuration.QPepConfig.General
	clientConfig := configuration.QPepConfig.Client

	// divert check
	if diverted {
		logger.Info("Diverted connection: %v:%v -> %v:%v", srcAddress, srcPort, dstAddress, dstPort)

		sessionHeader.SourceAddr = &net.TCPAddr{
			IP:   net.ParseIP(srcAddress),
			Port: srcPort,
		}
		sessionHeader.DestAddr = &net.TCPAddr{
			IP:   net.ParseIP(dstAddress),
			Port: dstPort,
		}

		if sessionHeader.DestAddr.IP.String() == clientConfig.GatewayHost {
			sessionHeader.Flags |= protocol.QPEP_LOCALSERVER_DESTINATION
		}

		logger.Info("Sending QPEP header to server, SourceAddr: %v / DestAddr: %v (Connection flags : %d %d)",
			sessionHeader.SourceAddr, sessionHeader.DestAddr,
			sessionHeader.Flags, sessionHeader.Flags&protocol.QPEP_LOCALSERVER_DESTINATION)

		_, err := quicStream.Write(sessionHeader.ToBytes())
		logger.OnError(err, "writing to quic stream")
	} else {
		if proxyRequest != nil {
			err = handleProxyedRequest(proxyRequest, &sessionHeader, tcpConn, quicStream)
			logger.OnError(err, "handling of proxy proxyRequest")
		}
	}

	//Proxy all stream content from quic to TCP and from TCP to quic
	logger.Debug("[%d] Stream Start", quicStream.ID())

	tqActiveFlag := atomic.Bool{}
	qtActiveFlag := atomic.Bool{}

	tqActiveFlag.Store(true)
	qtActiveFlag.Store(true)

	go handleTcpToQuic(ctx, &streamWait, quicStream, tcpConn, &qtActiveFlag, &tqActiveFlag)
	go handleQuicToTcp(ctx, &streamWait, tcpConn, quicStream, &qtActiveFlag, &tqActiveFlag)

	//we exit (and close the TCP connection) once both streams are done copying
	logger.Debug("[%d] Stream Wait", quicStream.ID())
	streamWait.Wait()
	logger.Info("[%d] Stream End (duration: %v)", quicStream.ID(), time.Now().Sub(startTime))

	if !generalConfig.MultiStream || (quicSession != nil && quicSession.IsClosed()) {
		// destroy the session so a new one is created next time
		newSessionLock.Lock()
		quicSession = nil
		newSessionLock.Unlock()
	}
}

// getQuicStream method handles the opening or reutilization of the quic session, and launches a new
// quic stream for communication
func getQuicStream(ctx context.Context) (backend.QuicBackendStream, error) {
	var err error
	var quicStream backend.QuicBackendStream = nil
	var localSession backend.QuicBackendConnection = nil

	newSessionLock.Lock()
	defer newSessionLock.Unlock()
	localSession = quicSession

	if localSession == nil || localSession.IsClosed() {
		// open a new quicSession (with all the TLS jazz)
		localSession, err = openQuicSession()
		// if we were unable to open a quic session, drop the TCP connection with RST
		if err != nil {
			return nil, err
		}

		quicSession = localSession
	}

	// if we allow for multiple streams in a session, try and open on the existing session
	if configuration.QPepConfig.General.MultiStream && localSession != nil {
		logger.Debug("Trying to open on existing session")
		quicStream, err = localSession.OpenStream(context.Background())
		if err == nil {
			return quicStream, nil
		}
		// if we weren't able to open a quicStream on that session (usually inactivity timeout), we can try to open a new session
		logger.OnError(err, "Unable to open new stream on existing Protocol session, closing session")
		quicStream = nil

		if quicSession != nil {
			quicSession.Close(0, "Stream could not be opened")
			quicSession = nil
		}

		return nil, errors.ErrFailedGatewayConnect
	}

	//Dial a stream to send writtenData on this new session
	quicStream, err = quicSession.OpenStream(ctx)
	// if we cannot open a stream on this session, send a TCP RST and let the client decide to try again
	logger.OnError(err, "Unable to open Protocol stream")
	if err != nil {
		return nil, err
	}
	return quicStream, nil
}

// handleProxyOpenConnection method wraps the logic for intercepting an http request with CONNECT or
// standard method to open the proxy connection correctly via the quic stream
func handleProxyOpenConnection(tcpConn net.Conn) (*http.Request, error) {
	// proxy check
	_ = tcpConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)) // not scaled as proxy connection is always local

	buf := bytes.NewBuffer(make([]byte, 0, INITIAL_BUFF_SIZE))
	n, err := io.Copy(buf, tcpConn)
	if n == 0 {
		logger.Error("Failed to copy request: %v\n", err)
		return nil, errors.ErrNonProxyableRequest
	}
	if err != nil {
		nErr, ok := err.(net.Error)
		if !ok || (ok && (!nErr.Timeout() && !nErr.Temporary())) {
			_ = tcpConn.Close()
			logger.Error("Failed to receive request: %v\n", err)
			return nil, errors.ErrNonProxyableRequest
		}
	}

	rd := bufio.NewReader(buf)
	req, err := http.ReadRequest(rd)
	if err != nil {
		_ = tcpConn.Close()
		logger.Error("Failed to parse request: %v\n", err)
		return nil, errors.ErrNonProxyableRequest
	}

	if checkProxyTestConnection(req.RequestURI) {
		var isProxyWorking = false
		if gateway.UsingProxy && gateway.ProxyAddress.String() == "http://"+tcpConn.LocalAddr().String() {
			isProxyWorking = true
		}

		t := http.Response{
			Status:        "200 Connection established",
			StatusCode:    http.StatusOK,
			Proto:         req.Proto,
			ProtoMajor:    req.ProtoMajor,
			ProtoMinor:    req.ProtoMinor,
			Body:          ioutil.NopCloser(bytes.NewBufferString("")),
			ContentLength: 0,
			Request:       req,
			Header: http.Header{
				gateway.QPEP_PROXY_HEADER: []string{fmt.Sprintf("%v", isProxyWorking)},
			},
		}

		t.Write(tcpConn)
		_ = tcpConn.Close()
		return nil, errors.ErrProxyCheckRequest
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
			return nil, errors.ErrNonProxyableRequest
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
		return nil, errors.ErrNonProxyableRequest
	}
	return req, nil
}

func handleProxyedRequest(req *http.Request, header *protocol.QPepHeader, tcpConn net.Conn, stream backend.QuicBackendStream) error {
	clientConfig := configuration.QPepConfig.Client

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

		logger.Info("HOST: %s", req.Host)

		// direct
		if header == nil {
			handleDirectConnection(tcpConn, req, fmt.Sprintf("%s:%d", address, port))
			break
		}

		header.DestAddr = &net.TCPAddr{
			IP:   address,
			Port: port,
		}

		if header.DestAddr.IP.String() == clientConfig.GatewayHost {
			header.Flags |= protocol.QPEP_LOCALSERVER_DESTINATION
		}

		headerData := header.ToBytes()
		logger.Info("Proxied connection flags : %d %d", header.Flags, header.Flags&protocol.QPEP_LOCALSERVER_DESTINATION)
		logger.Info("Sending QPEP header to server, SourceAddr: %v / DestAddr: %v / ID: %v", header.SourceAddr, header.DestAddr, stream.ID())
		logger.Info("QPEP header %v / ID: %v", headerData, stream.ID())

		_, err := stream.Write(headerData)
		if err != nil {
			_ = tcpConn.Close()
			logger.Error("Error writing to quic stream: %v", err)
			return errors.ErrFailed
		}

		logger.Debug("Sending captured %s request\n", req.Method)
		err = req.Write(stream)
		break

	case http.MethodConnect:
		address, port, proxyable := getAddressPortFromHost(req.Host)
		if !proxyable {
			panic("Should not happen as the handleProxyOpenConnection method checks the http request")
		}

		logger.Info("HOST: %s", req.Host)

		t := http.Response{
			Status:        "200 Connection established",
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

		if header == nil {
			handleDirectConnection(tcpConn, nil, fmt.Sprintf("%s:%d", address, port))
			break
		}

		header.DestAddr = &net.TCPAddr{
			IP:   address,
			Port: port,
		}

		if header.DestAddr.IP.String() == clientConfig.GatewayHost {
			header.Flags |= protocol.QPEP_LOCALSERVER_DESTINATION
		}
		logger.Info("Proxied connection flags : %d %d", header.Flags, header.Flags&protocol.QPEP_LOCALSERVER_DESTINATION)

		logger.Info("(Proxied) Sending QPEP header to server, SourceAddr: %v / DestAddr: %v / ID: %v", header.SourceAddr, header.DestAddr, stream.ID())

		_, err := stream.Write(header.ToBytes())
		if err != nil {
			_ = tcpConn.Close()
			logger.Error("Error writing to quic stream: %v", err)
			return errors.ErrFailed
		}
		break
	default:
		panic("Should not happen as the handleProxyOpenConnection method checks the http request method")
	}
	return nil
}

// handleTcpToQuic method implements the tcp connection to quic connection side of the connection
func handleTcpToQuic(ctx context.Context, streamWait *sync.WaitGroup, dst backend.QuicBackendStream, src net.Conn, qtFlag, tqFlag *atomic.Bool) {

	config := configuration.QPepConfig.Protocol

	buf := make([]byte, config.BufferSize*1024)
	written := int64(0)
	read := int64(0)

	logger.Debug("[%d] Stream T->Q start", dst.ID())

	tskKey := fmt.Sprintf("Tcp->Quic:%v", dst.ID())
	tsk := shared.StartRegion(tskKey)
	defer func() {
		if err := recover(); err != nil {
			logger.Error("ERR: %v", err)
			debug.PrintStack()
		}
		tsk.End()
		streamWait.Done()
		tqFlag.Store(false)
		logger.Info("[%d] Stream T->Q done [wr:%v rd:%d]", dst.ID(), written, read)
	}()

	pktPrefix := fmt.Sprintf("%v.client.tq", dst.ID())

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if dst.IsClosed() || !qtFlag.Load() {
			logger.Debug("[%v] T->Q CLOSE", dst.ID())
			return
		}

		wr, rd, err := backend.CopyBuffer(dst, src, buf, 100*time.Millisecond, pktPrefix)

		// update counters
		written += wr
		read += rd

		logger.Debug("[%d] T->Q: %v, %v", dst.ID(), wr, err)

		// stop / skip conditions
		if rd == 0 && err == nil {
			return
		}
		if err != nil {
			if err2, ok := err.(net.Error); ok && err2.Timeout() {
				continue
			}
			logger.Error("[%d] END T->Q: %v", dst.ID(), err)
			dst.AbortWrite(0)
			dst.AbortRead(0)
			return
		}
	}
	//logger.Info("Finished Copying TCP NetConn %s->%s, Stream ID %d\n", src.LocalAddr().String(), src.RemoteAddr().String(), dst.ID())
}

// handleQuicToTcp method implements the quic connection to tcp connection side of the connection
func handleQuicToTcp(ctx context.Context, streamWait *sync.WaitGroup, dst net.Conn, src backend.QuicBackendStream, qtFlag, tqFlag *atomic.Bool) {
	config := configuration.QPepConfig.Protocol

	buf := make([]byte, config.BufferSize*1024)
	written := int64(0)
	read := int64(0)

	logger.Debug("[%d] Stream Q->T start", src.ID())

	tskKey := fmt.Sprintf("Q->T:%v", src.ID())
	tsk := shared.StartRegion(tskKey)
	defer func() {
		if err := recover(); err != nil {
			logger.Error("ERR: %v", err)
			debug.PrintStack()
		}
		tsk.End()
		streamWait.Done()
		qtFlag.Store(false)
		logger.Info("[%d] Stream Q->T done [wr:%v rd:%d]", src.ID(), written, read)
	}()

	pktPrefix := fmt.Sprintf("%v.client.qt", src.ID())

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if src.IsClosed() || !tqFlag.Load() {
			logger.Error("[%v] Q->T CLOSE", src.ID())
			return
		}

		wr, rd, err := backend.CopyBuffer(dst, src, buf, 100*time.Millisecond, pktPrefix)

		// update counters
		written += wr
		read += rd

		logger.Debug("[%d] Q->T: %v, %v", src.ID(), wr, err)

		// stop / skip conditions
		if rd == 0 && err == nil {
			return
		}
		if err != nil {
			if err2, ok := err.(net.Error); ok && err2.Timeout() {
				continue
			}
			// closed tcp endpoint means its useless to go on with quic side
			logger.Error("[%d] END Q->T: %v", src.ID(), err)
			src.AbortWrite(0)
			src.AbortRead(0)
			return
		}
	}
}

func handleDirectConnection(conn net.Conn, req *http.Request, dest string) {
	defer conn.Close()

	logger.Info("Start direct connection: %v -> %v -> %v", conn.RemoteAddr(), conn.LocalAddr(), dest)
	defer logger.Info("End direct connection: %v -> %v -> %v", conn.RemoteAddr(), conn.LocalAddr(), dest)

	//
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 3 * time.Second,
		DualStack: true,
	}

	c, err := dialer.Dial("tcp", dest)
	if err != nil {
		logger.Error("ERROR: %v", err)
		return
	}
	defer c.Close()

	if req != nil {
		req.Write(c)
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer func() {
			_ = recover()
			wg.Done()
		}()
		conn.SetDeadline(time.Now().Add(10 * time.Second))
		c.SetDeadline(time.Now().Add(10 * time.Second))
		_, _ = io.Copy(conn, c)
	}()
	go func() {
		defer func() {
			_ = recover()
			wg.Done()
		}()
		conn.SetDeadline(time.Now().Add(10 * time.Second))
		c.SetDeadline(time.Now().Add(10 * time.Second))
		_, _ = io.Copy(c, conn)
	}()

	wg.Wait()
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
		if proxyable && port == 0 {
			port = 80
		}
	}
	return address, int(port), proxyable
}

var quicProvider backend.QuicBackend
var openSessionLock sync.Mutex

// openQuicSession implements the quic connection request to the qpep server
func openQuicSession() (backend.QuicBackendConnection, error) {
	configProto := configuration.QPepConfig.Protocol
	configSec := configuration.QPepConfig.Security
	clientConfig := configuration.QPepConfig.Client

	if quicProvider == nil {
		var ok bool
		quicProvider, ok = backend.Get(configProto.Backend)
		if !ok {
			panic(errors.ErrInvalidBackendSelected)
		}
	}

	openSessionLock.Lock()
	defer openSessionLock.Unlock()

	logger.Info("== Dialing Protocol Session: %s:%d ==\n", clientConfig.GatewayHost, clientConfig.GatewayPort)

	session, err := quicProvider.Dial(context.Background(),
		clientConfig.GatewayHost, clientConfig.GatewayPort,
		configSec.Certificate, configProto.CCAlgorithm, configProto.CCSlowstartAlgo,
		flags.Globals.Trace)

	if err != nil {
		logger.Error("== Unable to Dial Protocol Session: %v ==\n", err)
		return nil, errors.ErrFailedGatewayConnect
	}

	logger.Info("== Dialed Protocol Session: %s:%d (%v) ==\n", clientConfig.GatewayHost, clientConfig.GatewayPort,
		session)

	return session, nil
}
