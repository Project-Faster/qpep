package client

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/parvit/qpep/flags"
	"hash/crc64"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/parvit/qpep/backend"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/windivert"
	"golang.org/x/net/context"
)

const (
	BUFFER_SIZE = 512 * 1024

	DEBUG_DUMP_PACKETS = false
)

var (
	// proxyListener listener for the local http connections that get diverted or proxied to the quic server
	proxyListener net.Listener

	newSessionLock sync.RWMutex
	// quicSession listening quic connection to the server
	quicSession backend.QuicBackendConnection
)

type ReaderTimeout interface {
	io.Reader
	SetReadDeadline(time.Time) error
}

type WriterTimeout interface {
	io.Writer
	SetWriteDeadline(time.Time) error
}

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

		logger.Info("Sending QPEP header to server, SourceAddr: %v / DestAddr: %v", sessionHeader.SourceAddr, sessionHeader.DestAddr)
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
 
	tqActiveFlag := atomic.Bool{}
	qtActiveFlag := atomic.Bool{}

	tqActiveFlag.Store(true)
	qtActiveFlag.Store(true)

	go handleTcpToQuic(ctx, &streamWait, quicStream, tcpConn, &qtActiveFlag, &tqActiveFlag)
	go handleQuicToTcp(ctx, &streamWait, tcpConn, quicStream, &qtActiveFlag, &tqActiveFlag)

	//we exit (and close the TCP connection) once both streams are done copying
	logger.Info("== Stream %d Wait ==", quicStream.ID())
	streamWait.Wait()
	logger.Info("== Stream %d (duration: %v) End ==", quicStream.ID(), time.Now().Sub(startTime))

	if !ClientConfiguration.MultiStream || (quicSession != nil && quicSession.IsClosed()) {
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

		if quicSession != nil {
			quicSession.Close(0, "Stream could not be opened")
			quicSession = nil
		}

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
	_ = tcpConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)) // not scaled as proxy connection is always local

	buf := bytes.NewBuffer(make([]byte, 0, INITIAL_BUFF_SIZE))
	n, err := io.Copy(buf, tcpConn)
	if n == 0 {
		logger.Error("Failed to copy request: %v\n", err)
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

		logger.Info("HOST: %s", req.Host)

		header.DestAddr = &net.TCPAddr{
			IP:   address,
			Port: port,
		}

		if header.DestAddr.IP.String() == ClientConfiguration.GatewayHost {
			header.Flags |= shared.QPEP_LOCALSERVER_DESTINATION
		}

		headerData := header.ToBytes()
		logger.Info("Proxied connection flags : %d %d", header.Flags, header.Flags&shared.QPEP_LOCALSERVER_DESTINATION)
		logger.Info("Sending QPEP header to server, SourceAddr: %v / DestAddr: %v / ID: %v", header.SourceAddr, header.DestAddr, stream.ID())
		logger.Info("QPEP header %v / ID: %v", headerData, stream.ID())

		stream.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))

		logger.Info("QPEP header: %v", headerData)
		_, err := stream.Write(headerData)
		if err != nil {
			_ = tcpConn.Close()
			logger.Error("Error writing to quic stream: %v", err)
			return shared.ErrFailed
		}

		tcpConn.SetWriteDeadline(time.Now().Add(3 * time.Millisecond))

		logger.Info("Sending captured %s request\n", req.Method)
		err = req.Write(stream)
		//if err != nil {
		//	_ = tcpConn.Close()
		//	logger.Error("Error writing to tcp stream: %v", err)
		//	return shared.ErrFailed
		//}
		break

	case http.MethodConnect:
		address, port, proxyable := getAddressPortFromHost(req.Host)
		if !proxyable {
			panic("Should not happen as the handleProxyOpenConnection method checks the http request")
		}

		logger.Info("HOST: %s", req.Host)

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

		tcpConn.SetWriteDeadline(time.Now().Add(3 * time.Millisecond))

		t.Write(tcpConn)

		logger.Info("Proxied connection")
		logger.Info("Sending QPEP header to server, SourceAddr: %v / DestAddr: %v / ID: %v", header.SourceAddr, header.DestAddr, stream.ID())

		stream.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
		_, err := stream.Write(header.ToBytes())
		if err != nil {
			_ = tcpConn.Close()
			logger.Error("Error writing to quic stream: %v", err)
			return shared.ErrFailed
		}
		break
	default:
		panic("Should not happen as the handleProxyOpenConnection method checks the http request method")
	}
	return nil
}

// handleTcpToQuic method implements the tcp connection to quic connection side of the connection
func handleTcpToQuic(ctx context.Context, streamWait *sync.WaitGroup, dst backend.QuicBackendStream, src net.Conn, qtFlag, tqFlag *atomic.Bool) {

	buf := make([]byte, BUFFER_SIZE)
	written := int64(0)
	read := int64(0)

	logger.Info("== [%d] Stream TCP->Quic start ==", dst.ID())

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
		logger.Info("== Stream %v TCP->Quic [wr:%v rd:%d] done ==", dst.ID(), written, read)
	}()

	pktPrefix := fmt.Sprintf("%v.client.tq", dst.ID())
	pktcounter := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if dst.IsClosed() || !qtFlag.Load() {
			logger.Error("[%v] LINKED TQ CLOSE", dst.ID())
			return
		}

		wr, rd, err := copyBuffer(dst, src, buf, 100*time.Millisecond, 100*time.Millisecond, pktPrefix, &pktcounter)
		written += wr
		read += rd

		if rd == 0 && err == nil {
			return
		}

		logger.Debug("[%d] T->Q: %v, %v", dst.ID(), wr, err)

		if err != nil {
			if err2, ok := err.(net.Error); ok && err2.Timeout() {
				continue
			}
			logger.Debug("[%d] END T->Q: %v", dst.ID(), err)
			return
		}
	}
	//logger.Info("Finished Copying TCP Conn %s->%s, Stream ID %d\n", src.LocalAddr().String(), src.RemoteAddr().String(), dst.ID())
}

// handleQuicToTcp method implements the quic connection to tcp connection side of the connection
func handleQuicToTcp(ctx context.Context, streamWait *sync.WaitGroup, dst net.Conn, src backend.QuicBackendStream, qtFlag, tqFlag *atomic.Bool) {

	buf := make([]byte, BUFFER_SIZE)
	written := int64(0)
	read := int64(0)

	logger.Info("== [%d] Stream Quic->TCP start ==", src.ID())

	tskKey := fmt.Sprintf("Quic->Tcp:%v", src.ID())
	tsk := shared.StartRegion(tskKey)
	defer func() {
		if err := recover(); err != nil {
			logger.Error("ERR: %v", err)
			debug.PrintStack()
		}
		tsk.End()
		streamWait.Done()
		qtFlag.Store(false)
		logger.Info("== Stream %v Quic->TCP [wr:%v rd:%d] done ==", src.ID(), written, read)
	}()

	pktPrefix := fmt.Sprintf("%v.client.qt", src.ID())
	pktCounter := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if src.IsClosed() || !tqFlag.Load() {
			logger.Error("[%v] LINKED QT CLOSE", src.ID())
			return
		}

		wr, rd, err := copyBuffer(dst, src, buf, 100*time.Millisecond, 100*time.Millisecond, pktPrefix, &pktCounter)
		written += wr
		read += rd

		if rd == 0 && err == nil {
			return
		}

		logger.Debug("[%d] Q->T: %v, %v", src.ID(), wr, err)

		if err != nil {
			if err2, ok := err.(net.Error); ok && err2.Timeout() {
				continue
			}
			// closed tcp endpoint means its useless to go on with quic side
			logger.Debug("[%d] END Q->T: %v", src.ID(), err)
			return
		}
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
	if quicProvider == nil {
		var ok bool
		quicProvider, ok = backend.Get(shared.QPepConfig.Backend)
		if !ok {
			panic(shared.ErrInvalidBackendSelected)
		}
	}

	openSessionLock.Lock()
	defer openSessionLock.Unlock()

	logger.Info("== Dialing QUIC Session: %s:%d ==\n", ClientConfiguration.GatewayHost, ClientConfiguration.GatewayPort)
	session, err := quicProvider.Dial(context.Background(), ClientConfiguration.GatewayHost, ClientConfiguration.GatewayPort,
		shared.QPepConfig.Certificate, shared.QPepConfig.CCAlgorithm, shared.QPepConfig.CCSlowstartAlgo,
		flags.Globals.Trace)

	if err != nil {
		logger.Error("Unable to Dial QUIC Session: %v\n", err)
		return nil, shared.ErrFailedGatewayConnect
	}

	logger.Info("== Dialed QUIC Session: %s:%d (%v) ==\n", ClientConfiguration.GatewayHost, ClientConfiguration.GatewayPort,
		session)

	return session, nil
}

func copyBuffer(dst WriterTimeout, src ReaderTimeout, buf []byte, timeoutDst time.Duration, timeoutSrc time.Duration,
	prefix string, counter *int) (written, read int64, err error) {

	//limitSrc := io.LimitReader(src, BUFFER_SIZE)

	src.SetReadDeadline(time.Now().Add(timeoutDst))

	nr, er := src.Read(buf)

	if nr > 0 {
		read += int64(nr)

		if DEBUG_DUMP_PACKETS {
			dump, derr := os.Create(fmt.Sprintf("%s.%s.%d-rd.bin", prefix, shared.QPepConfig.Backend, *counter))
			if derr != nil {
				panic(derr)
			}
			dump.Write(buf[0:nr])
			go func() {
				dump.Sync()
				dump.Close()
			}()
			logger.Debug("[%d][%s] rd: %d (%v)", *counter, dump.Name(), nr, crc64.Checksum(buf[0:nr], crc64.MakeTable(crc64.ISO)))
		} else {
			logger.Debug("[%d][%s] rd: %d", *counter, prefix, nr)
		}

		offset := 0
		for offset < nr {
			dst.SetWriteDeadline(time.Now().Add(timeoutDst))

			nw, ew := dst.Write(buf[offset:nr])
			offset += nw
			written += int64(nw)
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = io.ErrUnexpectedEOF
					break
				}
			}
			if ew != nil {
				if err2, ok := ew.(net.Error); ok && err2.Timeout() {
					continue
				}
				err = ew
				break
			}
			//if nr != nw {
			//err = io.ErrShortWrite
			//}
		}

		if DEBUG_DUMP_PACKETS {
			dump, derr := os.Create(fmt.Sprintf("%s.%s.%d-wr.bin", prefix, shared.QPepConfig.Backend, *counter))
			if derr != nil {
				panic(derr)
			}
			dump.Write(buf[0:offset])
			go func() {
				dump.Sync()
				dump.Close()
			}()
			logger.Debug("[%d][%s] wr: %d (%v)", *counter, dump.Name(), offset, crc64.Checksum(buf[0:offset], crc64.MakeTable(crc64.ISO)))
		} else {
			logger.Debug("[%d][%s] wr: %d", *counter, prefix, offset)
		}
		*counter = *counter + 1

	} else {
		logger.Debug("[%d][%s] w,r: %d,%v **", *counter, prefix, 0, er)
	}

	if er != nil && er != io.EOF {
		err = er
	}
	return written, read, err
}
