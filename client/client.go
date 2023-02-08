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

	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/windivert"
	"golang.org/x/net/context"
)

const (
	// INITIAL_BUFF_SIZE indicates the initial receive buffer for connections
	INITIAL_BUFF_SIZE = int64(4096)
)

var (
	// redirected indicates if the connections are using the diverter for connection
	redirected = false
	// keepRedirectionRetries counter for the number of retries to keep trying to get a connection to server
	keepRedirectionRetries = shared.DEFAULT_REDIRECT_RETRIES

	// proxyListener listener for the local http connections that get diverted or proxied to the quic server
	proxyListener net.Listener
	// quicSession listening quic connection to the server
	quicSession quic.Connection

	// ClientConfiguration instance of the default configuration for the client
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
)

// ClientConfig struct that describes the parameter that can influence the behavior of the client
type ClientConfig struct {
	// ListenHost local address on which to listen for diverted / proxied connections
	ListenHost string
	// ListenPort local port on which to listen for diverted / proxied connections
	ListenPort int
	// GatewayHost remote address of the qpep server to which to establish quic connections
	GatewayHost string
	// GatewayPort remote port of the qpep server to which to establish quic connections
	GatewayPort int
	// RedirectedInterfaces list of ids of the interfaces that can be included for redirection
	RedirectedInterfaces []int64
	// APIPort Indicates the local/remote port of the API server (local will be 127.0.0.1:<APIPort>, remote <GatewayHost>:<APIPort>)
	APIPort int
	// QuicStreamTimeout Timeout in seconds for which to wait for a successful quic connection to the qpep server
	QuicStreamTimeout int
	// MultiStream indicates whether to enable the MultiStream option in quic-go library
	MultiStream bool
	// IdleTimeout Timeout after which, without activity, a connected quic stream is closed
	IdleTimeout time.Duration
	// MaxConnectionRetries Maximum number of tries for a qpep server connection after which the client is terminated
	MaxConnectionRetries int
	// PreferProxy If True, the first half of the MaxConnectionRetries uses the proxy instead of diverter, False is reversed
	PreferProxy bool
	// WinDivertThreads number of native threads that the windivert engine will be allowed to use
	WinDivertThreads int
	// Verbose outputs more log
	Verbose bool
}

// RunClient method executes the qpep in client mode and initializes its services
func RunClient(ctx context.Context, cancel context.CancelFunc) {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v", err)
			debug.PrintStack()
		}
		if proxyListener != nil {
			proxyListener.Close()
		}
		cancel()
	}()
	logger.Info("Starting TCP-QPEP Tunnel Listener")

	// update configuration from flags
	validateConfiguration()

	logger.Info("Binding to TCP %s:%d", ClientConfiguration.ListenHost, ClientConfiguration.ListenPort)
	var err error
	proxyListener, err = NewClientProxyListener("tcp", &net.TCPAddr{
		IP:   net.ParseIP(ClientConfiguration.ListenHost),
		Port: ClientConfiguration.ListenPort,
	})
	if err != nil {
		logger.Info("Encountered error when binding client proxy listener: %s", err)
		return
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go listenTCPConn(wg)
	go handleServices(ctx, cancel, wg)

	wg.Wait()
}

// handleServices method encapsulates the logic for checking the connection to the server
// by executing API calls
func handleServices(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup) {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v", err)
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
					logger.Info("Server returned public address %s\n", publicAddress)

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
				logger.Info("Error during statistics update from server\n")

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

// initialCheckConnection method checks whether the connections checks are to initialized or not
// and honors the PreferProxy setting
func initialCheckConnection() {
	if redirected || shared.UsingProxy {
		// no need to restart, already redirected
		return
	}

	keepRedirectionRetries = ClientConfiguration.MaxConnectionRetries // reset connection tries
	preferProxy := ClientConfiguration.PreferProxy

	if preferProxy {
		logger.Info("Proxy preference set, trying to connect...\n")
		initProxy()
		return
	}

	initDiverter()
}

// failedCheckConnection method handles the logic for switching between diverter and proxy (or viceversa if PreferProxy true)
// after half connection tries are failed, and stopping altogether if retries are exhausted
func failedCheckConnection() bool {
	maxRetries := ClientConfiguration.MaxConnectionRetries
	preferProxy := ClientConfiguration.PreferProxy

	keepRedirectionRetries--
	if preferProxy {
		// First half of tries with proxy, then diverter, then stop
		if shared.UsingProxy && keepRedirectionRetries < maxRetries/2 {
			stopProxy()
			logger.Info("Connection failed and half retries exhausted, trying with diverter\n")
			return !initDiverter()
		}
		if keepRedirectionRetries > 0 {
			logger.Info("Connection failed, keeping redirection active (retries left: %d)\n", keepRedirectionRetries)
			stopProxy()
			initProxy()
			return false
		}

		logger.Info("Connection failed and retries exhausted, redirection stopped\n")
		stopDiverter()
		return true
	}

	// First half of tries with diverter, then proxy, then stop
	if !shared.UsingProxy && keepRedirectionRetries < maxRetries/2 {
		stopDiverter()
		logger.Info("Connection failed and half retries exhausted, trying with proxy\n")
		initProxy()
		return false
	}
	if keepRedirectionRetries > 0 {
		logger.Info("Connection failed, keeping redirection active (retries left: %d)\n", keepRedirectionRetries)
		stopDiverter()
		initDiverter()
		return false
	}

	logger.Info("Connection failed and retries exhausted, redirection stopped\n")
	stopProxy()
	return true
}

// initDiverter method wraps the logic for initializing the windiverter engine, returns true if the diverter
// succeeded initialization and false otherwise
func initDiverter() bool {
	gatewayHost := ClientConfiguration.GatewayHost
	gatewayPort := ClientConfiguration.GatewayPort
	listenPort := ClientConfiguration.ListenPort
	threads := ClientConfiguration.WinDivertThreads
	listenHost := ClientConfiguration.ListenHost
	redirectedInterfaces := ClientConfiguration.RedirectedInterfaces

	// select an appropriate interface
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

	logger.Info("WinDivert: %v %v %v %v %v %v\n", gatewayHost, listenHost, gatewayPort, listenPort, threads, redirectedInetID)
	code := windivert.InitializeWinDivertEngine(gatewayHost, listenHost, gatewayPort, listenPort, threads, redirectedInetID)
	logger.Info("WinDivert code: %v\n", code)
	if code != windivert.DIVERT_OK {
		logger.Info("ERROR: Could not initialize WinDivert engine, code %d\n", code)
	}
	return code == windivert.DIVERT_OK
}

// stopDiverter method wraps the calls for stopping the diverter
func stopDiverter() {
	windivert.CloseWinDivertEngine()
	redirected = false
}

// initProxy method wraps the calls for initializing the proxy
func initProxy() {
	shared.UsingProxy = true
	shared.SetSystemProxy(true)
}

// stopProxy method wraps the calls for stopping the proxy
func stopProxy() {
	redirected = false
	shared.UsingProxy = false
	shared.SetSystemProxy(false)
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
		tcpConn.SetReadDeadline(time.Now().Add(1 * time.Second))

		buf := bytes.NewBuffer([]byte{})
		io.Copy(buf, tcpConn)

		rd := bufio.NewReader(buf)
		req, err := http.ReadRequest(rd)
		if err != nil {
			tcpConn.Close()
			logger.Info("Failed to parse request: %v\n", err)
			return
		}

		switch req.Method {
		case http.MethodGet:
			address, port, proxyable := getAddressPortFromHost(req.Host)
			if !proxyable {
				tcpConn.Close()
				logger.Info("Non proxyable request\n")
				return
			}

			sessionHeader.DestAddr = &net.TCPAddr{
				IP:   address,
				Port: port,
			}

			logger.Info("Proxied connection")
			logger.Info("Sending QUIC header to server, SourceAddr: %v / DestAddr: %v", sessionHeader.SourceAddr, sessionHeader.DestAddr)
			_, err := quicStream.Write(sessionHeader.ToBytes())
			if err != nil {
				logger.Info("Error writing to quic stream: %s", err.Error())
			}

			logger.Info("Sending captured GET request\n")
			err = req.Write(quicStream)
			if err != nil {
				logger.Info("Error writing to tcp stream: %s", err.Error())
			}
			break

		case http.MethodConnect:
			address, port, proxyable := getAddressPortFromHost(req.Host)
			if !proxyable {
				tcpConn.Close()
				logger.Info("Non proxyable request\n")
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

			logger.Info("Proxied connection")
			logger.Info("Sending QUIC header to server, SourceAddr: %v / DestAddr: %v", sessionHeader.SourceAddr, sessionHeader.DestAddr)
			_, err := quicStream.Write(sessionHeader.ToBytes())
			if err != nil {
				logger.Info("Error writing to quic stream: %s", err.Error())
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
			logger.Info("Proxy returns BadGateway\n")
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

	//Proxy all stream content from quic to TCP and from TCP to quic
	logger.Info("== Stream %d Start ==", quicStream.StreamID())
	go streamTCPtoQUIC(quicStream, tcpConn.(*net.TCPConn))
	go streamQUICtoTCP(tcpConn.(*net.TCPConn), quicStream)

	//we exit (and close the TCP connection) once both streams are done copying
	streamWait.Wait()
	tcpConn.(*net.TCPConn).SetLinger(3)
	tcpConn.Close()

	quicStream.CancelWrite(0)
	quicStream.CancelRead(0)
	quicStream.Close()
	logger.Info("== Stream %d Done ==", quicStream.StreamID())
}

// getAddressPortFromHost method returns an address splitted in the corresponding IP, port and if the indicated
// address can be used for proxying
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
			return session, nil
		} else {
			logger.Info("Failed to Open QUIC Session: %s\n    Retrying...\n", err)
		}
	}

	logger.Info("Max Retries Exceeded. Unable to Open QUIC Session: %s\n", err)
	return nil, err
}

// gatewayStatusCheck wraps the request for the /echo API to the api server
func gatewayStatusCheck(localAddr, apiAddr string, apiPort int) (bool, *api.EchoResponse) {
	if response := api.RequestEcho(localAddr, apiAddr, apiPort, true); response != nil {
		logger.Info("Gateway Echo OK\n")
		return true, response
	}
	logger.Info("Gateway Echo FAILED\n")
	return false, nil
}

// gatewayStatusCheck wraps the request for the /statistics/data API to the api server, and updates the local statistics
// with the ones received
func clientStatisticsUpdate(localAddr, apiAddr string, apiPort int, publicAddress string) bool {
	response := api.RequestStatistics(localAddr, apiAddr, apiPort, publicAddress)
	if response == nil {
		logger.Info("Statistics update failed, resetting connection status\n")
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

// validateConfiguration method handles the checking of the configuration values provided in the configuration files
// for the client mode
func validateConfiguration() {
	shared.AssertParamIP("listen host", shared.QPepConfig.ListenHost)
	shared.AssertParamPort("listen port", shared.QPepConfig.ListenPort)

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

	shared.AssertParamPort("api port", ClientConfiguration.APIPort)

	shared.AssertParamNumeric("max connection retries", ClientConfiguration.MaxConnectionRetries, 1, 300)

	shared.AssertParamHostsDifferent("hosts", ClientConfiguration.GatewayHost, ClientConfiguration.ListenHost)
	shared.AssertParamPortsDifferent("ports", ClientConfiguration.GatewayPort,
		ClientConfiguration.ListenPort, ClientConfiguration.APIPort)

	shared.AssertParamNumeric("auto-redirected interfaces", len(ClientConfiguration.RedirectedInterfaces), 1, 256)

	// validation ok
	logger.Info("Client configuration validation OK\n")
}
