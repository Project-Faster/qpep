package server

import (
	"context"
	"fmt"
	"github.com/Project-Faster/quic-go"
	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"io"
	"net"
	"runtime/debug"
	"sync"
	"time"
)

const (
	// INITIAL_BUFF_SIZE indicates the initial receive buffer for connections
	INITIAL_BUFF_SIZE = int64(4096)

	CHUNK_BUFF_SIZE = int64(409)
)

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

	srcLimit, okSrc := shared.GetAddressSpeedLimit(qpepHeader.SourceAddr.IP, true)
	dstLimit, okDst := shared.GetAddressSpeedLimit(qpepHeader.DestAddr.IP, false)
	if (okSrc && srcLimit == 0) || (okDst && dstLimit == 0) {
		logger.Info("Server speed limits blocked the connection, src:%v(%v) dst:%v(%v)", srcLimit, okSrc, dstLimit, okDst)
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
	proxyAddress := tcpConn.LocalAddr().String()

	api.Statistics.IncrementCounter(1.0, api.TOTAL_CONNECTIONS)
	api.Statistics.IncrementCounter(1.0, api.PERF_CONN, trackedAddress)
	defer func() {
		api.Statistics.DecrementCounter(1.0, api.PERF_CONN, trackedAddress)
		api.Statistics.DecrementCounter(1.0, api.TOTAL_CONNECTIONS)
	}()

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	var streamWait sync.WaitGroup
	streamWait.Add(2)

	go handleQuicToTcp(ctx, &streamWait, srcLimit, tcpConn, stream, proxyAddress, trackedAddress)
	go handleTcpToQuic(ctx, &streamWait, dstLimit, stream, tcpConn, trackedAddress)

	//we exit (and close the TCP connection) once both streams are done copying or timeout
	streamWait.Wait()
	tcpConn.Close()

	stream.CancelRead(0)
	stream.CancelWrite(0)
	stream.Close()
	logger.Info(">> Closing TCP Conn %s->%s\n", proxyAddress, tcpConn.RemoteAddr().String())
}

func handleQuicToTcp(ctx context.Context, streamWait *sync.WaitGroup, speedLimit int64,
	dst net.Conn, src quic.Stream, proxyAddress, trackedAddress string) {

	defer func() {
		_ = recover()

		api.Statistics.DeleteMappedAddress(proxyAddress)
		streamWait.Done()
	}()

	api.Statistics.SetMappedAddress(proxyAddress, trackedAddress)

	setLinger(dst)

	var timeoutCounter = 10
	var loopTimeout = 100 * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		tm := time.Now().Add(loopTimeout)
		_ = src.SetReadDeadline(tm)
		_ = src.SetWriteDeadline(tm)
		_ = dst.SetReadDeadline(tm)
		_ = dst.SetWriteDeadline(tm)

		var written int64 = 0
		var err error

		if speedLimit > 0 {
			var start = time.Now()
			var limit = start.Add(loopTimeout)
			written, err = io.Copy(dst, io.LimitReader(src, speedLimit/10))
			var end = limit.Sub(time.Now())

			logger.Debug("q -> t: %d / %v", written, end.Nanoseconds())
			time.Sleep(end)

		} else {
			written, err = io.Copy(dst, src)
			logger.Debug("q -> t: %d", written)
		}

		api.Statistics.IncrementCounter(float64(written), api.PERF_UP_COUNT, trackedAddress)

		if err != nil || written == 0 {
			if nErr, ok := err.(net.Error); timeoutCounter > 0 && ok && (nErr.Timeout() || nErr.Temporary()) {
				timeoutCounter--
				continue
			}
			//log.Printf("Error on Copy %s\n", err)
			logger.Debug("finish q -> t")
			return
		}
		timeoutCounter = 10
	}
}

func handleTcpToQuic(ctx context.Context, streamWait *sync.WaitGroup, speedLimit int64,
	dst quic.Stream, src net.Conn, trackedAddress string) {

	defer func() {
		_ = recover()

		streamWait.Done()
	}()

	setLinger(src)

	var loopTimeout = 100 * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		tm := time.Now().Add(loopTimeout)
		_ = src.SetReadDeadline(tm)
		_ = src.SetWriteDeadline(tm)
		_ = dst.SetReadDeadline(tm)
		_ = dst.SetWriteDeadline(tm)

		var written int64 = 0
		var err error

		if speedLimit > 0 {
			var start = time.Now()
			var limit = start.Add(loopTimeout)
			written, err = io.Copy(dst, io.LimitReader(src, speedLimit/10))
			var end = limit.Sub(time.Now())

			logger.Debug("t -> q: %d / %v", written, end.Nanoseconds())
			time.Sleep(end)

		} else {
			written, err = io.Copy(dst, src)
			logger.Debug("t -> q: %d", written)
		}

		api.Statistics.IncrementCounter(float64(written), api.PERF_DW_COUNT, trackedAddress)

		if err != nil || written == 0 {
			if nErr, ok := err.(net.Error); ok && (nErr.Timeout() || nErr.Temporary()) {
				continue
			}
			logger.Debug("finish t -> q")
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
