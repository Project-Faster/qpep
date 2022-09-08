package client

import (
	"crypto/tls"
	"io"
	"log"
	"net"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/windivert"
	"golang.org/x/net/context"
)

var (
	redirected             = false
	keepRedirectionRetries = shared.DEFAULT_REDIRECT_RETRIES

	proxyListener       net.Listener
	ClientConfiguration = ClientConfig{
		ListenHost: "0.0.0.0", ListenPort: 9443,
		GatewayHost: "198.56.1.10", GatewayPort: 443,
		RedirectedInterfaces: []int64{},
		QuicStreamTimeout:    2, MultiStream: shared.QuicConfiguration.MultiStream,
		MaxConnectionRetries: shared.DEFAULT_REDIRECT_RETRIES,
		IdleTimeout:          time.Duration(300) * time.Second,
		WinDivertThreads:     1,
		Verbose:              false,
	}
	quicSession             quic.Session
	QuicClientConfiguration = quic.Config{
		MaxIncomingStreams: 40000,
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
	WinDivertThreads     int
	Verbose              bool
}

func RunClient(ctx context.Context, cancel context.CancelFunc) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("PANIC: %v", err)
			debug.PrintStack()
		}
		if proxyListener != nil {
			proxyListener.Close()
		}
		cancel()
	}()
	log.Println("Starting TCP-QPEP Tunnel Listener")

	// update configuration from flags
	validateConfiguration()

	log.Printf("Binding to TCP %s:%d", ClientConfiguration.ListenHost, ClientConfiguration.ListenPort)
	var err error
	proxyListener, err = NewClientProxyListener("tcp", &net.TCPAddr{
		IP:   net.ParseIP(ClientConfiguration.ListenHost),
		Port: ClientConfiguration.ListenPort,
	})
	if err != nil {
		log.Printf("Encountered error when binding client proxy listener: %s", err)
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
			log.Printf("PANIC: %v", err)
			debug.PrintStack()
		}
		wg.Done()
		cancel()
	}()

	var connected = false
	var publicAddress = ""

	// start redirection right away because we normally expect the
	// connection with the server to be on already up
	successConnectionStartRedirection()

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
					keepRedirectionRetries = shared.QuicConfiguration.MaxConnectionRetries // reset connection tries
					log.Printf("Server returned public address %s\n", publicAddress)

				} else {
					// if connection is lost then keep the redirection active
					// for a certain number of retries then terminate to not keep
					// all the network blocked
					if failedConnectionCheckRedirection() {
						return
					}
				}
				continue
			}

			connected = clientStatisticsUpdate(localAddr, apiAddr, apiPort, publicAddress)
			if !connected {
				log.Printf("Error during statistics update from server\n")

				// if connection is lost then keep the redirection active
				// for a certain number of retries then terminate to not keep
				// all the network blocked
				if failedConnectionCheckRedirection() {
					return
				}
				connected = false
			}
		}
	}
}

func successConnectionStartRedirection() bool {

	gatewayHost := ClientConfiguration.GatewayHost
	gatewayPort := ClientConfiguration.GatewayPort
	redirectedInterfaces := ClientConfiguration.RedirectedInterfaces
	listenHost := ClientConfiguration.ListenHost
	listenPort := ClientConfiguration.ListenPort
	threads := ClientConfiguration.WinDivertThreads

	keepRedirectionRetries = shared.QuicConfiguration.MaxConnectionRetries // reset connection tries

	if redirected {
		// no need to restart, already redirected
		return true
	}
	code := windivert.InitializeWinDivertEngine(gatewayHost, listenHost, gatewayPort, listenPort, threads, redirectedInterfaces)
	if code != windivert.DIVERT_OK {
		log.Printf("ERROR: Could not initialize WinDivert engine, code %d\n", code)
		return true
	}
	return false
}

func failedConnectionCheckRedirection() bool {
	keepRedirectionRetries--
	if keepRedirectionRetries > 0 {
		log.Printf("Connection failed, keeping redirection active (retries left: %d)\n", keepRedirectionRetries)
		return false
	}

	log.Printf("Connection failed and retries exhausted, redirection stopped\n")
	redirected = false
	windivert.CloseWinDivertEngine()
	return true
}

func listenTCPConn(wg *sync.WaitGroup) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("PANIC: %v", err)
			debug.PrintStack()
		}
		wg.Done()
	}()
	for {
		conn, err := proxyListener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				log.Printf("Temporary error when accepting connection: %s", netErr)
			}
			log.Printf("Unrecoverable error while accepting connection: %s", err)
			return
		}

		go handleTCPConn(conn)
	}
}

func handleTCPConn(tcpConn net.Conn) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("PANIC: %v", err)
			debug.PrintStack()
		}
	}()
	log.Printf("Accepting TCP connection from %s with destination of %s", tcpConn.RemoteAddr().String(), tcpConn.LocalAddr().String())
	defer tcpConn.Close()
	var quicStream quic.Stream = nil
	// if we allow for multiple streams in a session, lets try and open on the existing session
	if ClientConfiguration.MultiStream {
		//if we have already opened a quic session, lets check if we've expired our stream
		if quicSession != nil {
			var err error
			log.Printf("Trying to open on existing session")
			quicStream, err = quicSession.OpenStream()
			// if we weren't able to open a quicStream on that session (usually inactivity timeout), we can try to open a new session
			if err != nil {
				log.Printf("Unable to open new stream on existing QUIC session: %s\n", err)
				quicStream = nil
			} else {
				log.Printf("Opened a new stream: %d", quicStream.StreamID())
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
			log.Printf("Unable to open QUIC stream: %s\n", err)
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

	diverted, srcPort, dstPort, srcAddress, dstAddress := windivert.GetConnectionStateData(sessionHeader.SourceAddr.Port)
	if diverted == windivert.DIVERT_OK {
		log.Printf("Diverted connection: %v:%v %v:%v", srcAddress, srcPort, dstAddress, dstPort)

		sessionHeader.SourceAddr = &net.TCPAddr{
			IP:   net.ParseIP(srcAddress),
			Port: srcPort,
		}
		sessionHeader.DestAddr = &net.TCPAddr{
			IP:   net.ParseIP(dstAddress),
			Port: dstPort,
		}
	}

	log.Printf("Sending QUIC header to server, SourceAddr: %v / DestAddr: %v", sessionHeader.SourceAddr, sessionHeader.DestAddr)

	_, err := quicStream.Write(sessionHeader.ToBytes())
	if err != nil {
		log.Printf("Error writing to quic stream: %s", err.Error())
	}

	streamQUICtoTCP := func(dst *net.TCPConn, src quic.Stream) {
		_, err := io.Copy(dst, src)
		dst.SetLinger(3)
		dst.Close()
		//src.CancelRead(1)
		//src.Close()
		if err != nil {
			log.Printf("Error on Copy %s", err)
		}
		streamWait.Done()
	}

	streamTCPtoQUIC := func(dst quic.Stream, src *net.TCPConn) {
		_, err := io.Copy(dst, src)
		src.SetLinger(3)
		src.Close()
		//src.CloseWrite()
		//dst.CancelWrite(1)
		//dst.Close()
		if err != nil {
			log.Printf("Error on Copy %s", err)
		}
		streamWait.Done()
	}

	//Proxy all stream content from quic to TCP and from TCP to quic
	go streamTCPtoQUIC(quicStream, tcpConn.(*net.TCPConn))
	go streamQUICtoTCP(tcpConn.(*net.TCPConn), quicStream)

	//we exit (and close the TCP connection) once both streams are done copying
	streamWait.Wait()
	quicStream.Close()
	log.Printf("Done sending data on %d", quicStream.StreamID())
}

func openQuicSession() (quic.Session, error) {
	var err error
	var session quic.Session
	tlsConf := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"qpep"}}
	gatewayPath := ClientConfiguration.GatewayHost + ":" + strconv.Itoa(ClientConfiguration.GatewayPort)
	quicClientConfig := QuicClientConfiguration
	log.Printf("Dialing QUIC Session: %s\n", gatewayPath)
	for i := 0; i < ClientConfiguration.MaxConnectionRetries; i++ {
		session, err = quic.DialAddr(gatewayPath, tlsConf, &quicClientConfig)
		if err == nil {
			return session, nil
		} else {
			log.Printf("Failed to Open QUIC Session: %s\n    Retrying...\n", err)
		}
	}

	log.Printf("Max Retries Exceeded. Unable to Open QUIC Session: %s\n", err)
	return nil, err
}

func gatewayStatusCheck(localAddr, apiAddr string, apiPort int) (bool, *api.EchoResponse) {
	if response := api.RequestEcho(localAddr, apiAddr, apiPort, true); response != nil {
		log.Printf("Gateway Echo OK\n")
		return true, response
	}
	log.Printf("Gateway Echo FAILED\n")
	return false, nil
}

func clientStatisticsUpdate(localAddr, apiAddr string, apiPort int, publicAddress string) bool {
	response := api.RequestStatistics(localAddr, apiAddr, apiPort, publicAddress)
	if response == nil {
		log.Printf("Statistics update failed, resetting connection status\n")
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
	ClientConfiguration.GatewayHost = shared.QuicConfiguration.GatewayIP
	ClientConfiguration.GatewayPort = shared.QuicConfiguration.GatewayPort
	ClientConfiguration.APIPort = shared.QuicConfiguration.GatewayAPIPort
	ClientConfiguration.ListenHost, ClientConfiguration.RedirectedInterfaces = shared.GetDefaultLanListeningAddress(shared.QuicConfiguration.ListenIP)
	ClientConfiguration.ListenPort = shared.QuicConfiguration.ListenPort
	ClientConfiguration.MaxConnectionRetries = shared.QuicConfiguration.MaxConnectionRetries
	ClientConfiguration.MultiStream = shared.QuicConfiguration.MultiStream
	ClientConfiguration.WinDivertThreads = shared.QuicConfiguration.WinDivertThreads
	ClientConfiguration.Verbose = shared.QuicConfiguration.Verbose

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
	log.Printf("Client configuration validation OK\n")
}
