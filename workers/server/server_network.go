package server

import (
	"context"
	"fmt"
	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/backend"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"io"
	"net"
	"runtime/debug"
	"sync"
	"time"
)

const (
	BUFFER_SIZE = 512 * 1024
)

var (
	// quicSession listening quic connection to the server
	quicProvider backend.QuicBackend
	quicListener backend.QuicBackendConnection
)

// listenQuicSession handles accepting the sessions and the launches goroutines to actually serve them
func listenQuicSession(address string, port int) {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v\n", err)
			debug.PrintStack()
		}
	}()
	if quicProvider == nil {
		var ok bool
		quicProvider, ok = backend.Get(shared.QPepConfig.Backend)
		if !ok {
			panic(shared.ErrInvalidBackendSelected)
		}
	}

	var err error
	quicListener, err = quicProvider.Listen(context.Background(), address, port)
	if err != nil {
		logger.Error("Unrecoverable error while listening for QUIC connections: %s\n", err)
		return
	}

	for {
		quicSession, err := quicListener.AcceptConnection(context.Background())
		if err != nil {
			logger.Error("Unrecoverable error while accepting QUIC session: %s\n", err)
			return
		}
		go func() {
			listenQuicConn(quicSession)
		}()
	}
}

// listenQuicConn handles opened quic sessions and accepts connections in goroutines to actually serve them
func listenQuicConn(quicSession backend.QuicBackendConnection) {
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
		go func() {
			logger.Info("== [%d] Stream Start ==", stream.ID())
			handleQuicStream(stream)
			logger.Info("== [%d] Stream End ==", stream.ID())
		}()
	}
}

// handleQuicStream handles a quic stream connection and bridges to the standard tcp for the common internet
func handleQuicStream(quicStream backend.QuicBackendStream) {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v\n", err)
			debug.PrintStack()
		}
	}()

	qpepHeader, err := shared.QPepHeaderFromBytes(quicStream)
	if err != nil {
		logger.Info("Unable to decode QPEP header: %s\n", err)
		_ = quicStream.Close()
		return
	}

	srcLimit, okSrc := shared.GetAddressSpeedLimit(qpepHeader.SourceAddr.IP, true)
	dstLimit, okDst := shared.GetAddressSpeedLimit(qpepHeader.DestAddr.IP, false)
	if (okSrc && srcLimit == 0) || (okDst && dstLimit == 0) {
		logger.Info("Server speed limits blocked the connection, src:%v(%v) dst:%v(%v)", srcLimit, okSrc, dstLimit, okDst)
		_ = quicStream.Close()
		return
	}

	logger.Info("[%d] Connection flags : %d %v", quicStream.ID(), qpepHeader.Flags, qpepHeader.Flags&shared.QPEP_LOCALSERVER_DESTINATION != 0)

	// To support the server being behind a private NAT (external gateway address != local listening address)
	// we dial the listening address when the connection is directed at the non-local API server
	destAddress := qpepHeader.DestAddr.String()
	if qpepHeader.Flags&shared.QPEP_LOCALSERVER_DESTINATION != 0 {
		logger.Info("[%d] Local connection to server", quicStream.ID())
		destAddress = fmt.Sprintf("127.0.0.1:%d", qpepHeader.DestAddr.Port)
	}

	logger.Debug("[%d] >> Opening TCP Conn to dest:%s, src:%s\n", quicStream.ID(), destAddress, qpepHeader.SourceAddr)
	dial := &net.Dialer{
		LocalAddr:     &net.TCPAddr{IP: net.ParseIP(ServerConfiguration.ListenHost)},
		Timeout:       shared.GetScaledTimeout(10, time.Second),
		KeepAlive:     shared.GetScaledTimeout(15, time.Second),
		DualStack:     true,
		FallbackDelay: 10 * time.Millisecond,
	}
	tcpConn, err := dial.Dial("tcp", destAddress)
	if err != nil {
		logger.Error("[%d] Unable to open TCP connection from QPEP quicStream: %s\n", quicStream.ID(), err)
		quicStream.Close()

		shared.ScaleUpTimeout()
		return
	}
	logger.Info(">> [%d] Opened TCP Conn %s -> %s\n", quicStream.ID(), qpepHeader.SourceAddr, destAddress)

	trackedAddress := qpepHeader.SourceAddr.IP.String()
	proxyAddress := tcpConn.LocalAddr().String()

	api.Statistics.IncrementCounter(1.0, api.TOTAL_CONNECTIONS)
	api.Statistics.IncrementCounter(1.0, api.PERF_CONN, trackedAddress)
	defer func() {
		api.Statistics.DecrementCounter(1.0, api.PERF_CONN, trackedAddress)
		api.Statistics.DecrementCounter(1.0, api.TOTAL_CONNECTIONS)
	}()

	ctx, _ := context.WithCancel(context.Background())

	var streamWait sync.WaitGroup
	streamWait.Add(2)

	go handleQuicToTcp(ctx, &streamWait, srcLimit, tcpConn, quicStream, proxyAddress, trackedAddress)
	go handleTcpToQuic(ctx, &streamWait, dstLimit, quicStream, tcpConn, trackedAddress)
	//go connectionActivityTimer(quicStream, tcpConn, &activityRX, &activityTX, cancel)

	//we exit (and close the TCP connection) once both streams are done copying or timeout
	logger.Info("== Stream %d Wait ==", quicStream.ID())
	streamWait.Wait()
	logger.Info("== Stream %d WaitEnd ==", quicStream.ID())

	tcpConn.Close()

	<-time.After(5 * time.Second) // linger for a certain amount of time before definetely closing

	quicStream.Close()
}

func handleQuicToTcp(ctx context.Context, streamWait *sync.WaitGroup, speedLimit int64,
	dst net.Conn, src backend.QuicBackendStream, proxyAddress, trackedAddress string) {

	defer func() {
		if err := recover(); err != nil {
			logger.Error("ERR: %v", err)
			debug.PrintStack()
		}

		api.Statistics.DeleteMappedAddress(proxyAddress)
		streamWait.Done()
		logger.Info("== [%d] Stream Quic->TCP done ==", src.ID())
	}()

	api.Statistics.SetMappedAddress(proxyAddress, trackedAddress)

	setLinger(dst)

	timeoutCounter := 0
	wr, err := io.Copy(dst, io.LimitReader(src, BUFFER_SIZE*2))
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		//logger.Info("[%d] Q->T: %v: %v", src.ID(), activityFlag, *activityFlag)

		_ = src.SetReadDeadline(time.Now().Add(1 * time.Second))
		_ = dst.SetDeadline(time.Now().Add(1 * time.Second))
		if speedLimit == 0 {
			wr, err = io.Copy(dst, io.LimitReader(src, BUFFER_SIZE))
		} else {
			var now = time.Now()
			wr, err = io.Copy(dst, io.LimitReader(src, speedLimit))

			var wait = time.Until(now.Add(1 * time.Second))
			time.Sleep(wait)
		}

		if wr == 0 {
			timeoutCounter++
			if timeoutCounter > 5 {
				return
			}
		} else {
			timeoutCounter = 0
		}

		if err != nil {
			if err, ok := err.(net.Error); ok && err.Timeout() {
				continue
			}
			return
		}
	}
}

func handleTcpToQuic(ctx context.Context, streamWait *sync.WaitGroup, speedLimit int64,
	dst backend.QuicBackendStream, src net.Conn, trackedAddress string) {

	defer func() {
		if err := recover(); err != nil {
			logger.Error("ERR: %v", err)
			debug.PrintStack()
		}

		streamWait.Done()
		logger.Info("== [%d] Stream TCP->Quic done ==", dst.ID())
	}()

	setLinger(src)

	timeoutCounter := 0
	wr, err := io.Copy(dst, io.LimitReader(src, BUFFER_SIZE*2))
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		//logger.Info("[%d] T->Q: %v: %v", dst.ID(), activityFlag, *activityFlag)

		_ = src.SetDeadline(time.Now().Add(1 * time.Second))
		_ = dst.SetWriteDeadline(time.Now().Add(1 * time.Second))
		if speedLimit == 0 {
			wr, err = io.Copy(dst, io.LimitReader(src, BUFFER_SIZE))
		} else {
			var now = time.Now()
			wr, err = io.Copy(dst, io.LimitReader(src, speedLimit))

			var wait = time.Until(now.Add(1 * time.Second))
			time.Sleep(wait)
		}

		if wr == 0 {
			timeoutCounter++
			if timeoutCounter > 5 {
				return
			}
		} else {
			timeoutCounter = 0
		}

		if err != nil {
			if err, ok := err.(net.Error); ok && err.Timeout() {
				continue
			}
			return
		}
	}
}

func setLinger(c net.Conn) {
	if conn, ok := c.(*net.TCPConn); ok {
		err1 := conn.SetLinger(1)
		logger.OnError(err1, "error on setLinger")
	}
}
