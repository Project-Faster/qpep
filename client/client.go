package client

import (
	"bufio"
	"bytes"
	//"github.com/lucas-clemente/quic-go/logging"
	//"github.com/lucas-clemente/quic-go/qlog"
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

	"github.com/Project-Faster/quic-go"

	"github.com/parvit/qpep/api"
	. "github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/windivert"
	"golang.org/x/net/context"
)

const (
	INITIAL_BUFF_SIZE = int64(4096)
)

var (
	redirected             = false
	keepRedirectionRetries = shared.DEFAULT_REDIRECT_RETRIES

	proxyListener       net.Listener
	ClientConfiguration = ClientConfig{
		ListenHost: "0.0.0.0", ListenPort: 9443,
		GatewayHost: "198.56.1.10", GatewayPort: 443,
		RedirectedInterfaces: []int64{},
		QuicStreamTimeout:    2, MultiStream: shared.QPepConfig.MultiStream,
		MaxConnectionRetries: shared.DEFAULT_REDIRECT_RETRIES,
		IdleTimeout:          time.Duration(300) * time.Second,
		WinDivertThreads:     1,
		Verbose:              false,
	}
	quicSession             quic.Connection
	QuicClientConfiguration = quic.Config{
		MaxIncomingStreams:      40000,
		DisablePathMTUDiscovery: true,
		//		Tracer: qlog.NewTracer(func(_ logging.Perspective, connID []byte) io.WriteCloser {
		//			filename := fmt.Sprintf("client_%x.qlog", connID)
		//			f, err := os.Create(filename)
		//			if err != nil {
		//				log.Fatal(err)
		//			}
		//			Info("Creating qlog file %s.\n", filename)
		//			return &shared.QLogWriter{Writer: bufio.NewWriter(f)}
		//		}),
	}
)

type ClientConfig struct {
	ListenHost           string
	ListenPort           int
	GatewayHost          string
	GatewayPort          int
	RedirectedInterfaces []int64
	APIPort              int
	QuicStreamTimeout    int
	MultiStream          bool
	IdleTimeout          time.Duration
	MaxConnectionRetries int
	PreferProxy          bool
	WinDivertThreads     int
	Verbose              bool
}

func RunClient(ctx context.Context, cancel context.CancelFunc) {
	defer func() {
		if err := recover(); err != nil {
			Info("PANIC: %v", err)
			debug.PrintStack()
		}
		if proxyListener != nil {
			proxyListener.Close()
		}
		cancel()
	}()
	Info("Starting TCP-QPEP Tunnel Listener")

	// update configuration from flags
	validateConfiguration()

	Info("Binding to TCP %s:%d", ClientConfiguration.ListenHost, ClientConfiguration.ListenPort)
	var err error
	proxyListener, err = NewClientProxyListener("tcp", &net.TCPAddr{
		IP:   net.ParseIP(ClientConfiguration.ListenHost),
		Port: ClientConfiguration.ListenPort,
	})
	if err != nil {
		Info("Encountered error when binding client proxy listener: %s", err)
		return
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go listenTCPConn(wg)
	go handleServices(ctx, cancel, wg)

	wg.Wait()
}

func handleServices(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup) {
	defer func() {
		if err := recover(); err != nil {
			Info("PANIC: %v", err)
			debug.PrintStack()
		}
		wg.Done()
		cancel()
	}()

	var connected = false
	var publicAddress = ""

	// start redirection right away because we normally expect the
	// connection with the server to be on already up
	initialCheckConnection()

	// Update loop
	for {
		select {
		case <-ctx.Done():
			proxyListener.Close()
			return

		case <-time.After(1 * time.Second):
			localAddr := ClientConfiguration.ListenHost
			apiAddr := ClientConfiguration.GatewayHost
			apiPort := ClientConfiguration.APIPort
			if !connected {
				if ok, response := gatewayStatusCheck(localAddr, apiAddr, apiPort); ok {
					publicAddress = response.Address
					connected = true
					Info("Server returned public address %s\n", publicAddress)

				} else {
					// if connection is lost then keep the redirection active
					// for a certain number of retries then terminate to not keep
					// all the network blocked
					if failedCheckConnection() {
						return
					}
				}
				continue
			}

			connected = clientStatisticsUpdate(localAddr, apiAddr, apiPort, publicAddress)
			if !connected {
				Info("Error during statistics update from server\n")

				// if connection is lost then keep the redirection active
				// for a certain number of retries then terminate to not keep
				// all the network blocked
				if failedCheckConnection() {
					return
				}
				connected = false
			}
		}
	}
}

func initialCheckConnection() bool {
	if redirected || shared.UsingProxy {
		// no need to restart, already redirected
		return true
	}

	keepRedirectionRetries = ClientConfiguration.MaxConnectionRetries // reset connection tries
	preferProxy := ClientConfiguration.PreferProxy

	if preferProxy {
		Info("Proxy preference set, trying to connect...\n")
		initProxy()
		return false
	}

	return initDiverter()
}

func failedCheckConnection() bool {
	maxRetries := ClientConfiguration.MaxConnectionRetries
	preferProxy := ClientConfiguration.PreferProxy

	keepRedirectionRetries--
	if preferProxy {
		// First half of tries with proxy, then diverter, then stop
		if shared.UsingProxy && keepRedirectionRetries < maxRetries/2 {
			stopProxy()
			Info("Connection failed and half retries exhausted, trying with diverter\n")
			return initDiverter()
		}
		if keepRedirectionRetries > 0 {
			Info("Connection failed, keeping redirection active (retries left: %d)\n", keepRedirectionRetries)
			stopProxy()
			initProxy()
			return false
		}

		Info("Connection failed and retries exhausted, redirection stopped\n")
		stopDiverter()
		return true
	} else {
		// First half of tries with diverter, then proxy, then stop
		if !shared.UsingProxy && keepRedirectionRetries < maxRetries/2 {
			stopDiverter()
			Info("Connection failed and half retries exhausted, trying with proxy\n")
			initProxy()
			return false
		}
		if keepRedirectionRetries > 0 {
			Info("Connection failed, keeping redirection active (retries left: %d)\n", keepRedirectionRetries)
			stopDiverter()
			initDiverter()
			return false
		}

		Info("Connection failed and retries exhausted, redirection stopped\n")
		stopProxy()
		return true
	}
}

func initDiverter() bool {
	gatewayHost := ClientConfiguration.GatewayHost
	gatewayPort := ClientConfiguration.GatewayPort
	listenPort := ClientConfiguration.ListenPort
	threads := ClientConfiguration.WinDivertThreads
	listenHost := ClientConfiguration.ListenHost
	redirectedInterfaces := ClientConfiguration.RedirectedInterfaces

	var redirectedInetID int64 = 0
	for _, id := range redirectedInterfaces {
		inet, _ := net.InterfaceByIndex(int(id))

		addresses, _ := inet.Addrs()
		for _, addr := range addresses {
			if strings.Contains(addr.String(), listenHost) {
				redirectedInetID = id
				break
			}
		}
	}

	Info("WinDivert: %v %v %v %v %v %v\n", gatewayHost, listenHost, gatewayPort, listenPort, threads, redirectedInetID)
	code := windivert.InitializeWinDivertEngine(gatewayHost, listenHost, gatewayPort, listenPort, threads, redirectedInetID)
	Info("WinDivert code: %v\n", code)
	if code != windivert.DIVERT_OK {
		Info("ERROR: Could not initialize WinDivert engine, code %d\n", code)
	}
	return code != windivert.DIVERT_OK
}

func stopDiverter() {
	windivert.CloseWinDivertEngine()
	redirected = false
}

func initProxy() {
	shared.UsingProxy = true
	shared.SetSystemProxy(true)
}

func stopProxy() {
	redirected = false
	shared.UsingProxy = false
	shared.SetSystemProxy(false)
}

func listenTCPConn(wg *sync.WaitGroup) {
	defer func() {
		if err := recover(); err != nil {
			Info("PANIC: %v", err)
			debug.PrintStack()
		}
		wg.Done()
	}()
	for {
		conn, err := proxyListener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				Info("Temporary error when accepting connection: %s", netErr)
			}
			Info("Unrecoverable error while accepting connection: %s", err)
			return
		}

		go handleTCPConn(conn)
	}
}

func handleTCPConn(tcpConn net.Conn) {
	defer func() {
		if err := recover(); err != nil {
			Info("PANIC: %v", err)
			debug.PrintStack()
		}
	}()
	Info("Accepting TCP connection from %s with destination of %s", tcpConn.RemoteAddr().String(), tcpConn.LocalAddr().String())
	defer tcpConn.Close()

	var quicStream quic.Stream = nil
	// if we allow for multiple streams in a session, lets try and open on the existing session
	if ClientConfiguration.MultiStream {
		//if we have already opened a quic session, lets check if we've expired our stream
		if quicSession != nil {
			var err error
			Info("Trying to open on existing session")
			quicStream, err = quicSession.OpenStream()
			// if we weren't able to open a quicStream on that session (usually inactivity timeout), we can try to open a new session
			if err != nil {
				Info("Unable to open new stream on existing QUIC session: %s\n", err)
				quicStream = nil
			} else {
				Info("Opened a new stream: %d", quicStream.StreamID())
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
			Info("Unable to open QUIC stream: %s\n", err)
			return
		}
	}
	defer quicStream.Close()

	//We want to wait for both the upstream and downstream to finish so we'll set a wait group for the threads
	var streamWait sync.WaitGroup
	streamWait.Add(2)

	//Set our custom header to the QUIC session so the server can generate the correct TCP handshake on the other side
	sessionHeader := shared.QpepHeader{
		SourceAddr: tcpConn.RemoteAddr().(*net.TCPAddr),
		DestAddr:   tcpConn.LocalAddr().(*net.TCPAddr),
	}

	// divert check
	diverted, srcPort, dstPort, srcAddress, dstAddress := windivert.GetConnectionStateData(sessionHeader.SourceAddr.Port)
	if diverted == windivert.DIVERT_OK {
		Info("Diverted connection: %v:%v %v:%v", srcAddress, srcPort, dstAddress, dstPort)

		sessionHeader.SourceAddr = &net.TCPAddr{
			IP:   net.ParseIP(srcAddress),
			Port: srcPort,
		}
		sessionHeader.DestAddr = &net.TCPAddr{
			IP:   net.ParseIP(dstAddress),
			Port: dstPort,
		}

		Info("Sending QUIC header to server, SourceAddr: %v / DestAddr: %v", sessionHeader.SourceAddr, sessionHeader.DestAddr)
		_, err := quicStream.Write(sessionHeader.ToBytes())
		if err != nil {
			Info("Error writing to quic stream: %s", err.Error())
		}
	} else {
		tcpConn.SetReadDeadline(time.Now().Add(1 * time.Second))

		buf := bytes.NewBuffer([]byte{})
		io.Copy(buf, tcpConn)

		rd := bufio.NewReader(buf)
		req, err := http.ReadRequest(rd)
		if err != nil {
			tcpConn.Close()
			Info("Failed to parse request: %v\n", err)
			return
		}

		switch req.Method {
		case http.MethodGet:
			address, port, proxyable := getAddressPortFromHost(req.Host)
			if !proxyable {
				tcpConn.Close()
				Info("Non proxyable request\n")
				return
			}

			sessionHeader.DestAddr = &net.TCPAddr{
				IP:   address,
				Port: port,
			}

			Info("Proxied connection")
			Info("Sending QUIC header to server, SourceAddr: %v / DestAddr: %v", sessionHeader.SourceAddr, sessionHeader.DestAddr)
			_, err := quicStream.Write(sessionHeader.ToBytes())
			if err != nil {
				Info("Error writing to quic stream: %s", err.Error())
			}

			Info("Sending captured GET request\n")
			err = req.Write(quicStream)
			if err != nil {
				Info("Error writing to tcp stream: %s", err.Error())
			}
			break

		case http.MethodConnect:
			address, port, proxyable := getAddressPortFromHost(req.Host)
			if !proxyable {
				tcpConn.Close()
				Info("Non proxyable request\n")
				return
			}

			sessionHeader.DestAddr = &net.TCPAddr{
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

			Info("Proxied connection")
			Info("Sending QUIC header to server, SourceAddr: %v / DestAddr: %v", sessionHeader.SourceAddr, sessionHeader.DestAddr)
			_, err := quicStream.Write(sessionHeader.ToBytes())
			if err != nil {
				Info("Error writing to quic stream: %s", err.Error())
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
			Info("Proxy returns BadGateway\n")
			return
		}
	}

	ctx, _ := context.WithTimeout(context.Background(), ClientConfiguration.IdleTimeout)

	streamQUICtoTCP := func(dst *net.TCPConn, src quic.Stream) {
		defer func() {
			_ = recover()

			streamWait.Done()
		}()

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

			written, err := io.Copy(dst, io.LimitReader(src, buffSize))
			if err != nil || written == 0 {
				if nErr, ok := err.(net.Error); ok && (nErr.Timeout() || nErr.Temporary()) {
					continue
				}
				//Info("Error on Copy %s\n", err)
				break
			}

			buffSize = int64(written * 2)
			if buffSize < INITIAL_BUFF_SIZE {
				buffSize = INITIAL_BUFF_SIZE
			}
		}
		//Info("Finished Copying Stream ID %d, TCP Conn %s->%s\n", src.StreamID(), dst.LocalAddr().String(), dst.RemoteAddr().String())
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

			written, err := io.Copy(dst, io.LimitReader(src, buffSize))
			if err != nil || written == 0 {
				if nErr, ok := err.(net.Error); ok && (nErr.Timeout() || nErr.Temporary()) {
					continue
				}
				//Info("Error on Copy %s\n", err)
				break
			}

			buffSize = int64(written * 2)
			if buffSize < INITIAL_BUFF_SIZE {
				buffSize = INITIAL_BUFF_SIZE
			}
		}
		//Info("Finished Copying TCP Conn %s->%s, Stream ID %d\n", src.LocalAddr().String(), src.RemoteAddr().String(), dst.StreamID())
	}

	//Proxy all stream content from quic to TCP and from TCP to quic
	Info("== Stream %d Start ==", quicStream.StreamID())
	go streamTCPtoQUIC(quicStream, tcpConn.(*net.TCPConn))
	go streamQUICtoTCP(tcpConn.(*net.TCPConn), quicStream)

	//we exit (and close the TCP connection) once both streams are done copying
	streamWait.Wait()
	tcpConn.(*net.TCPConn).SetLinger(3)
	tcpConn.Close()

	quicStream.CancelWrite(0)
	quicStream.CancelRead(0)
	quicStream.Close()
	Info("== Stream %d Done ==", quicStream.StreamID())
}

func getAddressPortFromHost(host string) (net.IP, int, bool) {
	var proxyable = false
	var port int64 = 0
	var address net.IP
	urlParts := strings.Split(host, ":")
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

func openQuicSession() (quic.Connection, error) {
	var err error
	var session quic.Connection
	tlsConf := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"qpep"}}
	gatewayPath := ClientConfiguration.GatewayHost + ":" + strconv.Itoa(ClientConfiguration.GatewayPort)
	quicClientConfig := QuicClientConfiguration
	Info("Dialing QUIC Session: %s\n", gatewayPath)
	for i := 0; i < ClientConfiguration.MaxConnectionRetries; i++ {
		session, err = quic.DialAddr(gatewayPath, tlsConf, &quicClientConfig)
		if err == nil {
			return session, nil
		} else {
			Info("Failed to Open QUIC Session: %s\n    Retrying...\n", err)
		}
	}

	Info("Max Retries Exceeded. Unable to Open QUIC Session: %s\n", err)
	return nil, err
}

func gatewayStatusCheck(localAddr, apiAddr string, apiPort int) (bool, *api.EchoResponse) {
	if response := api.RequestEcho(localAddr, apiAddr, apiPort, true); response != nil {
		Info("Gateway Echo OK\n")
		return true, response
	}
	Info("Gateway Echo FAILED\n")
	return false, nil
}

func clientStatisticsUpdate(localAddr, apiAddr string, apiPort int, publicAddress string) bool {
	response := api.RequestStatistics(localAddr, apiAddr, apiPort, publicAddress)
	if response == nil {
		Info("Statistics update failed, resetting connection status\n")
		return false
	}

	for _, stat := range response.Data {
		value, err := strconv.ParseFloat(stat.Value, 64)
		if err != nil {
			continue
		}
		api.Statistics.SetCounter(value, stat.Name)
	}
	return true
}

func validateConfiguration() {
	// copy values for client configuration
	ClientConfiguration.GatewayHost = shared.QPepConfig.GatewayHost
	ClientConfiguration.GatewayPort = shared.QPepConfig.GatewayPort
	ClientConfiguration.APIPort = shared.QPepConfig.GatewayAPIPort
	ClientConfiguration.ListenHost, ClientConfiguration.RedirectedInterfaces = shared.GetDefaultLanListeningAddress(
		shared.QPepConfig.ListenHost, shared.QPepConfig.GatewayHost)
	ClientConfiguration.ListenPort = shared.QPepConfig.ListenPort
	ClientConfiguration.MaxConnectionRetries = shared.QPepConfig.MaxConnectionRetries
	ClientConfiguration.MultiStream = shared.QPepConfig.MultiStream
	ClientConfiguration.WinDivertThreads = shared.QPepConfig.WinDivertThreads
	ClientConfiguration.PreferProxy = shared.QPepConfig.PreferProxy
	ClientConfiguration.Verbose = shared.QPepConfig.Verbose

	// panic if configuration is inconsistent
	shared.AssertParamIP("gateway host", ClientConfiguration.GatewayHost)
	shared.AssertParamPort("gateway port", ClientConfiguration.GatewayPort)

	shared.AssertParamIP("listen host", ClientConfiguration.ListenHost)
	shared.AssertParamPort("listen port", ClientConfiguration.ListenPort)

	shared.AssertParamPort("api port", ClientConfiguration.APIPort)

	shared.AssertParamNumeric("max connection retries", ClientConfiguration.MaxConnectionRetries, 1, 300)

	shared.AssertParamHostsDifferent("hosts", ClientConfiguration.GatewayHost, ClientConfiguration.ListenHost)
	shared.AssertParamPortsDifferent("ports", ClientConfiguration.GatewayPort,
		ClientConfiguration.ListenPort, ClientConfiguration.APIPort)

	shared.AssertParamNumeric("auto-redirected interfaces", len(ClientConfiguration.RedirectedInterfaces), 1, 256)

	// validation ok
	Info("Client configuration validation OK\n")
}
