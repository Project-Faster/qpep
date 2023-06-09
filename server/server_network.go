package server

import (
	"context"
	"fmt"
	"github.com/Project-Faster/quic-go"
	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"net"
	"runtime/debug"
	"sync"
	"time"
)

const (
	BUFFER_SIZE = 512 * 1024

	ACTIVITY_RX_FLAG = "activity_rx"
	ACTIVITY_TX_FLAG = "activity_tx"
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
		go func() {
			listenQuicConn(quicSession)
		}()
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
		go func() {
			logger.Info("== Stream %d Start ==", stream.StreamID())
			handleQuicStream(stream)
			logger.Info("== Stream %d End ==", stream.StreamID())
		}()
	}
}

func connectionActivityTimer(dst quic.Stream, src net.Conn, flag_rx, flag_tx *bool, cancelFunc context.CancelFunc) {
	if flag_tx == nil || flag_rx == nil {
		return
	}
	<-time.After(ServerConfiguration.IdleTimeout)
	if !*flag_rx && !*flag_tx {
		logger.Info("[%v] connection canceled for inactivity", dst.StreamID())
		cancelFunc()
		dst.Close()
		src.Close()
		return
	}
	go connectionActivityTimer(dst, src, flag_rx, flag_tx, cancelFunc)
}

// handleQuicStream handles a quic stream connection and bridges to the standard tcp for the common internet
func handleQuicStream(quicStream quic.Stream) {
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

	logger.Info("[%d] Connection flags : %d %v", quicStream.StreamID(), qpepHeader.Flags, qpepHeader.Flags&shared.QPEP_LOCALSERVER_DESTINATION != 0)

	// To support the server being behind a private NAT (external gateway address != local listening address)
	// we dial the listening address when the connection is directed at the non-local API server
	destAddress := qpepHeader.DestAddr.String()
	if qpepHeader.Flags&shared.QPEP_LOCALSERVER_DESTINATION != 0 {
		logger.Info("[%d] Local connection to server", quicStream.StreamID())
		destAddress = fmt.Sprintf("127.0.0.1:%d", qpepHeader.DestAddr.Port)
	}

	logger.Debug("[%d] >> Opening TCP Conn to dest:%s, src:%s\n", quicStream.StreamID(), destAddress, qpepHeader.SourceAddr)
	dial := &net.Dialer{
		LocalAddr:     &net.TCPAddr{IP: net.ParseIP(ServerConfiguration.ListenHost)},
		Timeout:       shared.GetScaledTimeout(10, time.Second),
		KeepAlive:     shared.GetScaledTimeout(15, time.Second),
		DualStack:     true,
		FallbackDelay: 10 * time.Millisecond,
	}
	tcpConn, err := dial.Dial("tcp", destAddress)
	if err != nil {
		logger.Error("[%d] Unable to open TCP connection from QPEP quicStream: %s\n", quicStream.StreamID(), err)
		quicStream.Close()

		shared.ScaleUpTimeout()
		return
	}
	logger.Info(">> [%d] Opened TCP Conn %s -> %s\n", quicStream.StreamID(), qpepHeader.SourceAddr, destAddress)

	trackedAddress := qpepHeader.SourceAddr.IP.String()
	proxyAddress := tcpConn.LocalAddr().String()

	api.Statistics.IncrementCounter(1.0, api.TOTAL_CONNECTIONS)
	api.Statistics.IncrementCounter(1.0, api.PERF_CONN, trackedAddress)
	defer func() {
		api.Statistics.DecrementCounter(1.0, api.PERF_CONN, trackedAddress)
		api.Statistics.DecrementCounter(1.0, api.TOTAL_CONNECTIONS)
	}()

	ctx, cancel := context.WithCancel(context.Background())

	var streamWait sync.WaitGroup
	streamWait.Add(2)

	var activityRX, activityTX = true, true
	ctx = context.WithValue(ctx, ACTIVITY_RX_FLAG, &activityRX)
	ctx = context.WithValue(ctx, ACTIVITY_TX_FLAG, &activityTX)
	defer func() {
		// terminate activity timer
		activityTX = false
		activityRX = false
	}()

	go handleQuicToTcp(ctx, &streamWait, srcLimit, tcpConn, quicStream, proxyAddress, trackedAddress)
	go handleTcpToQuic(ctx, &streamWait, dstLimit, quicStream, tcpConn, trackedAddress)
	go connectionActivityTimer(quicStream, tcpConn, &activityRX, &activityTX, cancel)

	//we exit (and close the TCP connection) once both streams are done copying or timeout
	logger.Info("== Stream %d Wait ==", quicStream.StreamID())
	streamWait.Wait()
	logger.Info("== Stream %d WaitEnd ==", quicStream.StreamID())

	quicStream.Close()
	tcpConn.Close()
}

func handleQuicToTcp(ctx context.Context, streamWait *sync.WaitGroup, speedLimit int64,
	dst net.Conn, src quic.Stream, proxyAddress, trackedAddress string) {

	defer func() {
		_ = recover()

		dst.Close()
		api.Statistics.DeleteMappedAddress(proxyAddress)
		streamWait.Done()
		logger.Info("== Stream %v Quic->TCP done ==", src.StreamID())
	}()

	api.Statistics.SetMappedAddress(proxyAddress, trackedAddress)

	var activityFlag, ok = ctx.Value(ACTIVITY_RX_FLAG).(*bool)
	if !ok {
		panic("No activity flag set")
	}

	setLinger(dst)

	var loopTimeout = 1 * time.Second
	var tempBuffer []byte
	if speedLimit > 0 {
		tempBuffer = make([]byte, speedLimit)
	} else {
		tempBuffer = make([]byte, BUFFER_SIZE)
	}

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

		var start = time.Now()
		var limit = start.Add(loopTimeout)
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
		for read > 0 {
			*activityFlag = true
			tm = time.Now().Add(loopTimeout)
			_ = dst.SetReadDeadline(tm)
			_ = dst.SetWriteDeadline(tm)

			written, err = dst.Write(tempBuffer[:read])
			if nErr, ok := err_t.(net.Error); ok && nErr.Timeout() {
				*activityFlag = false
				<-time.After(1 * time.Millisecond)
				continue
			}
			read = 0
		}
		var end = limit.Sub(time.Now())
		if speedLimit != 0 {
			time.Sleep(end)
		}

		if written > 0 {
			*activityFlag = true
			api.Statistics.IncrementCounter(float64(written), api.PERF_UP_COUNT, trackedAddress)
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

func handleTcpToQuic(ctx context.Context, streamWait *sync.WaitGroup, speedLimit int64,
	dst quic.Stream, src net.Conn, trackedAddress string) {

	defer func() {
		_ = recover()

		dst.Close()
		streamWait.Done()
		logger.Info("== Stream %v TCP->Quic done ==", dst.StreamID())
	}()

	var activityFlag, ok = ctx.Value(ACTIVITY_TX_FLAG).(*bool)
	if !ok {
		panic("No activity flag set")
	}

	setLinger(src)

	var loopTimeout = 1 * time.Second
	var tempBuffer []byte
	if speedLimit > 0 {
		tempBuffer = make([]byte, speedLimit)
	} else {
		tempBuffer = make([]byte, BUFFER_SIZE)
	}

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

		var start = time.Now()
		var limit = start.Add(loopTimeout)
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
		for read > 0 {
			*activityFlag = true
			tm = time.Now().Add(loopTimeout)
			_ = dst.SetReadDeadline(tm)
			_ = dst.SetWriteDeadline(tm)

			written, err = dst.Write(tempBuffer[:read])
			if nErr, ok := err_t.(net.Error); ok && nErr.Timeout() {
				*activityFlag = false
				<-time.After(1 * time.Millisecond)
				continue
			}
			read = 0
		}
		var end = limit.Sub(time.Now())
		if speedLimit != 0 {
			time.Sleep(end)
		}

		if written > 0 {
			*activityFlag = true
			api.Statistics.IncrementCounter(float64(written), api.PERF_DW_COUNT, trackedAddress)
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
