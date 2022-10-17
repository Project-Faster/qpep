package client

import (
	"crypto/tls"
	"io"
	"net"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"

	"github.com/parvit/qpep/api"
	. "github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"github.com/parvit/qpep/windivert"
	"golang.org/x/net/context"
)

var (
	proxyListener       net.Listener
	ClientConfiguration = ClientConfig{
		ListenHost: "0.0.0.0", ListenPort: 9443,
		GatewayHost: "198.56.1.10", GatewayPort: 443,
		RedirectedInterfaces: []int64{},
		QuicStreamTimeout:    2, MultiStream: shared.QPepConfig.MultiStream,
		ConnectionRetries: 3,
		IdleTimeout:       time.Duration(300) * time.Second,
		WinDivertThreads:  1,
		Verbose:           false,
	}
	quicSession             quic.Connection
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
	ConnectionRetries    int
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

	// Initialize WinDivertEngine
	windivert.EnableDiverterLogging(ClientConfiguration.Verbose)

	gatewayHost := ClientConfiguration.GatewayHost
	gatewayPort := ClientConfiguration.GatewayPort
	redirectedInterfaces := ClientConfiguration.RedirectedInterfaces
	listenHost := ClientConfiguration.ListenHost
	listenPort := ClientConfiguration.ListenPort
	threads := ClientConfiguration.WinDivertThreads

	code := windivert.InitializeWinDivertEngine(gatewayHost, listenHost, gatewayPort, listenPort, threads, redirectedInterfaces)

	if code != windivert.DIVERT_OK {
		windivert.CloseWinDivertEngine()

		Info("ERROR: Could not initialize WinDivert engine, code %d\n", code)
		cancel()
		return
	}

	// Start listener
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

	go ListenTCPConn()

	var connected = false
	var publicAddress = ""

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
					connected = true
					publicAddress = response.Address
					Info("Server returned public address %s\n", publicAddress)
				}
				continue
			}

			connected = clientStatisticsUpdate(localAddr, apiAddr, apiPort, publicAddress)
		}
	}
}

func ListenTCPConn() {
	defer func() {
		if err := recover(); err != nil {
			Info("PANIC: %v", err)
			debug.PrintStack()
		}
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
	}

	Info("Sending QUIC header to server, SourceAddr: %v / DestAddr: %v", sessionHeader.SourceAddr, sessionHeader.DestAddr)

	_, err := quicStream.Write(sessionHeader.ToBytes())
	if err != nil {
		Info("Error writing to quic stream: %s", err.Error())
	}

	streamQUICtoTCP := func(dst *net.TCPConn, src quic.Stream) {
		_, err := io.Copy(dst, src)
		dst.SetLinger(3)
		dst.Close()
		//src.CancelRead(1)
		//src.Close()
		if err != nil {
			Info("Error on Copy %s", err)
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
			Info("Error on Copy %s", err)
		}
		streamWait.Done()
	}

	//Proxy all stream content from quic to TCP and from TCP to quic
	go streamTCPtoQUIC(quicStream, tcpConn.(*net.TCPConn))
	go streamQUICtoTCP(tcpConn.(*net.TCPConn), quicStream)

	//we exit (and close the TCP connection) once both streams are done copying
	streamWait.Wait()
	quicStream.Close()
	Info("Done sending data on %d", quicStream.StreamID())
}

func openQuicSession() (quic.Connection, error) {
	var err error
	var session quic.Connection
	tlsConf := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"qpep"}}
	gatewayPath := ClientConfiguration.GatewayHost + ":" + strconv.Itoa(ClientConfiguration.GatewayPort)
	quicClientConfig := QuicClientConfiguration
	Info("Dialing QUIC Session: %s\n", gatewayPath)
	for i := 0; i < ClientConfiguration.ConnectionRetries; i++ {
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
	ClientConfiguration.ListenHost, ClientConfiguration.RedirectedInterfaces = shared.GetDefaultLanListeningAddress(shared.QPepConfig.ListenHost)
	ClientConfiguration.ListenPort = shared.QPepConfig.ListenPort
	ClientConfiguration.MultiStream = shared.QPepConfig.MultiStream
	ClientConfiguration.WinDivertThreads = shared.QPepConfig.WinDivertThreads
	ClientConfiguration.Verbose = shared.QPepConfig.Verbose

	// panic if configuration is inconsistent
	shared.AssertParamIP("gateway host", ClientConfiguration.GatewayHost)
	shared.AssertParamPort("gateway port", ClientConfiguration.GatewayPort)

	shared.AssertParamIP("listen host", ClientConfiguration.ListenHost)
	shared.AssertParamPort("listen port", ClientConfiguration.ListenPort)

	shared.AssertParamPort("api port", ClientConfiguration.APIPort)

	shared.AssertParamHostsDifferent("hosts", ClientConfiguration.GatewayHost, ClientConfiguration.ListenHost)
	shared.AssertParamPortsDifferent("ports", ClientConfiguration.GatewayPort,
		ClientConfiguration.ListenPort, ClientConfiguration.APIPort)

	shared.AssertParamNumeric("auto-redirected interfaces", len(ClientConfiguration.RedirectedInterfaces), 1, 256)

	// validation ok
	Info("Client configuration validation OK\n")
}
