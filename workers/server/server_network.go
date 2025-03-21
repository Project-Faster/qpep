package server

import (
	"context"
	"fmt"
	"github.com/Project-Faster/qpep/api"
	"github.com/Project-Faster/qpep/backend"
	"github.com/Project-Faster/qpep/shared"
	"github.com/Project-Faster/qpep/shared/configuration"
	"github.com/Project-Faster/qpep/shared/errors"
	"github.com/Project-Faster/qpep/shared/flags"
	"github.com/Project-Faster/qpep/shared/logger"
	"github.com/Project-Faster/qpep/shared/protocol"
	"io"
	"net"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// quicSession listening quic connection to the server
	quicProvider backend.QuicBackend
	quicListener backend.QuicBackendConnection
)

type ReaderTimeout interface {
	io.Reader
	SetReadDeadline(time.Time) error
}

type WriterTimeout interface {
	io.Writer
	SetWriteDeadline(time.Time) error
}

// listenQuicSession handles accepting the sessions and the launches goroutines to actually serve them
func listenQuicSession(ctx context.Context, cancel context.CancelFunc, address string, port int) {
	defer func() {
		if err := recover(); err != nil {
			logger.Error("PANIC: %v\n%v\n", err, string(debug.Stack()))
		}
		cancel()
	}()
	configSec := configuration.QPepConfig.Security
	configProto := configuration.QPepConfig.Protocol

	if quicProvider == nil {
		var ok bool
		quicProvider, ok = backend.Get(configProto.Backend)
		if !ok {
			panic(errors.ErrInvalidBackendSelected)
		}
	}

	var err error
	quicListener, err = quicProvider.Listen(ctx, address, port,
		configSec.Certificate, configSec.PrivateKey,
		configProto.CCAlgorithm, configProto.CCSlowstartAlgo,
		flags.Globals.Trace)

	if err != nil {
		logger.Error("Unrecoverable error while listening for Protocol connections: %s\n", err)
		var errPtr = ctx.Value("lastError").(*error)
		*errPtr = err
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			break
		}

		quicSession, err := quicListener.AcceptConnection(ctx)
		if err != nil {
			logger.Error("Unrecoverable error while accepting Protocol session: %s\n", err)
			var errPtr = ctx.Value("lastError").(*error)
			*errPtr = err
			return
		}
		go func() {
			listenQuicConn(quicSession)
		}()
	}
}

func setLinger(c net.Conn) {
	if conn, ok := c.(*net.TCPConn); ok {
		err1 := conn.SetLinger(1)
		logger.OnError(err1, "error on setLinger")
	}
}

// listenQuicConn handles opened quic sessions and accepts connections in goroutines to actually serve them
func listenQuicConn(quicConn backend.QuicBackendConnection) {
	defer func() {
		if err := recover(); err != nil {
			logger.Info("PANIC: %v\n", err)
			debug.PrintStack()
		}
	}()
	for {
		quicStream, err := quicConn.AcceptStream(context.Background())
		if err != nil {
			logger.Error("Unrecoverable error while accepting Protocol stream: %s\n", err)
			quicConn.Close(0, "")
			return
		}
		go func(stream backend.QuicBackendStream) {
			if stream == nil {
				return
			}
			defer func() {
				if err := recover(); err != nil {
					logger.Info("PANIC: %v\n", err)
					debug.PrintStack()
				}
			}()
			tskKey := fmt.Sprintf("QuicStream:%v", stream.ID())
			tsk := shared.StartRegion(tskKey)
			defer tsk.End()
			//for i := 0; i < 10; i++ {
			//	connCounter := api.Statistics.GetCounter("", api.TOTAL_CONNECTIONS)
			//	if connCounter >= 16 {
			//		logger.Info("[%d] Stream Queued (current: %d / max: %d)", stream.ID(), connCounter, 16)
			//		<-time.After(100 * time.Millisecond)
			//		continue
			//	}
			logger.Debug("[%d] Stream Start", stream.ID())
			handleQuicStream(stream)
			logger.Debug("[%d] Stream End", stream.ID())
			//	return
			//}
			//logger.Info("[%d] Session Rejected for too many connections", stream.ID())
			//_ = stream.Close()
			//_ = quicConn.Close(1, "Session Rejected for too many connections")
		}(quicStream)
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

	qpepHeader, err := protocol.QPepHeaderFromBytes(quicStream)
	if err != nil {
		logger.Error("Unable to decode QPEP header: %s\n", err)
		closeStreamNow(quicStream)
		return
	}

	srcLimit, okSrc := configuration.GetAddressSpeedLimit(qpepHeader.SourceAddr.IP, true)
	dstLimit, okDst := configuration.GetAddressSpeedLimit(qpepHeader.DestAddr.IP, false)
	if (okSrc && srcLimit == 0) || (okDst && dstLimit == 0) {
		logger.Info("Server speed limits blocked the connection, src:%v(%v) dst:%v(%v)", srcLimit, okSrc, dstLimit, okDst)
		closeStreamNow(quicStream)
		return
	}

	logger.Debug("[%d] Connection flags : %d %v", quicStream.ID(), qpepHeader.Flags, qpepHeader.Flags&protocol.QPEP_LOCALSERVER_DESTINATION != 0)

	// To support the server being behind a private NAT (external gateway address != local listening address)
	// we dial the listening address when the connection is directed at the non-local API server
	srcAddress := qpepHeader.SourceAddr.String()
	destAddress := qpepHeader.DestAddr.String()
	localSrcAddress := &net.TCPAddr{IP: net.ParseIP(configuration.QPepConfig.Server.ExternalListeningAddress)}

	if qpepHeader.Flags&protocol.QPEP_LOCALSERVER_DESTINATION != 0 {
		logger.Debug("[%d] Local connection to server", quicStream.ID())
		destAddress = fmt.Sprintf("127.0.0.1:%d", qpepHeader.DestAddr.Port)
		localSrcAddress = nil
	}

	tskKey := fmt.Sprintf("TCP-Dial:%v:%v", quicStream.ID(), destAddress)
	tsk := shared.StartRegion(tskKey)
	logger.Debug("[%d] >> Opening TCP NetConn to dest:%s, src:%s\n", quicStream.ID(), destAddress, qpepHeader.SourceAddr)
	dial := &net.Dialer{
		Timeout:       30 * time.Second,
		Deadline:      time.Now().Add(30 * time.Second),
		KeepAlive:     -1,
		DualStack:     false,
		FallbackDelay: -1,
	}
	if localSrcAddress != nil {
		dial.LocalAddr = localSrcAddress
	}

	tcpConn, err := dial.Dial("tcp", destAddress)
	tsk.End()
	if err != nil {
		logger.Error("[%d] Unable to open TCP connection from QPEP quicStream: %s\n", quicStream.ID(), err)
		closeStreamNow(quicStream)
		return
	}
	logger.Info("[%d] Opened TCP NetConn %s -> %s\n", quicStream.ID(), qpepHeader.SourceAddr, destAddress)

	startTime := time.Now()
	tqActiveFlag := atomic.Bool{}
	qtActiveFlag := atomic.Bool{}

	tqActiveFlag.Store(true)
	qtActiveFlag.Store(true)

	//setLinger(tcpConn)

	api.Statistics.SetMappedAddress(srcAddress, destAddress)
	api.Statistics.IncrementCounter(1.0, api.TOTAL_CONNECTIONS)
	api.Statistics.IncrementCounter(1.0, api.PERF_CONN, srcAddress)
	defer func() {
		api.Statistics.DecrementCounter(1.0, api.PERF_CONN, srcAddress)
		api.Statistics.DecrementCounter(1.0, api.TOTAL_CONNECTIONS)
		api.Statistics.DeleteMappedAddress(srcAddress)
	}()

	ctx, _ := context.WithCancel(context.Background())

	var streamWait sync.WaitGroup
	streamWait.Add(2)

	go handleQuicToTcp(ctx, &streamWait, srcLimit, tcpConn, quicStream, &qtActiveFlag, &tqActiveFlag)
	go handleTcpToQuic(ctx, &streamWait, dstLimit, quicStream, tcpConn, &qtActiveFlag, &tqActiveFlag)

	//we exit (and close the TCP connection) once both streams are done copying or timeout
	logger.Debug("[%v] Stream Wait", quicStream.ID())
	streamWait.Wait()

	tcpConn.Close()
	quicStream.Close()

	logger.Info("[%d] Closed TCP NetConn %s -> %s [%d] (%v)\n", quicStream.ID(), qpepHeader.SourceAddr, destAddress, quicStream.ID(), time.Now().Sub(startTime))
}

func handleQuicToTcp(ctx context.Context, streamWait *sync.WaitGroup, speedLimit int64, dst net.Conn, src backend.QuicBackendStream, qtFlag, tqFlag *atomic.Bool) {

	written := int64(0)
	read := int64(0)

	buf := make([]byte, configuration.QPepConfig.Protocol.BufferSize*1024)
	periodStart := time.Now()
	periodWritten := int64(0)

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
		logger.Info("[%d] Stream Q->T [wr:%v rd:%d] done", src.ID(), written, read)
	}()

	pktPrefix := fmt.Sprintf("%v.server.qt", src.ID())

	i := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if src.IsClosed() || !tqFlag.Load() {
			logger.Debug("[%v] T->Q CLOSE (%v %v %v)", src.ID(), src.IsClosed(), !tqFlag.Load(), src.Sync())
			return
		}

		i++
		var now = time.Now()
		wr, rd, err := backend.CopyBuffer(dst, src, buf, 100*time.Millisecond, pktPrefix)

		written += wr
		read += rd

		// obey speed limit if set
		if speedLimit > 0 && wr >= speedLimit {
			var wait = time.Until(now.Add(1 * time.Second))
			time.Sleep(wait)
		}

		logger.Debug("[%d] Q->T: %v, %v", src.ID(), wr, err)

		// stop / skip conditions
		if rd == 0 && err == nil {
			return
		}
		if err != nil {
			if err2, ok := err.(net.Error); ok && err2.Timeout() {
				continue
			}
			logger.Error("[%d] STREAM ERR Q->T: %v", src.ID(), err)
			closeStreamNow(src)
			return
		}

		// update speed limit
		if speedLimit > 0 {
			periodWritten += wr
			if periodWritten > speedLimit {
				periodWritten = 0
				<-time.After(time.Until(periodStart.Add(1 * time.Second)))
				periodStart = time.Now()
			}
		}
	}
}

func handleTcpToQuic(ctx context.Context, streamWait *sync.WaitGroup, speedLimit int64, dst backend.QuicBackendStream, src net.Conn, qtFlag, tqFlag *atomic.Bool) {

	written := int64(0)
	read := int64(0)

	buf := make([]byte, configuration.QPepConfig.Protocol.BufferSize*1024)
	periodStart := time.Now()
	periodWritten := int64(0)

	logger.Debug("[%d] Stream T->Q start", dst.ID())

	tskKey := fmt.Sprintf("T->Q:%v", dst.ID())
	tsk := shared.StartRegion(tskKey)
	defer func() {
		if err := recover(); err != nil {
			logger.Error("ERR: %v", err)
			debug.PrintStack()
		}
		tsk.End()
		tqFlag.Store(false)
		streamWait.Done()
		logger.Info("[%d] Stream T->Q [wr:%v rd:%d] done", dst.ID(), written, read)
	}()

	pktPrefix := fmt.Sprintf("%v.server.tq", dst.ID())

	i := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if dst.IsClosed() || !qtFlag.Load() {
			logger.Error("[%v] T->Q CLOSE (%v %v %v)", dst.ID(), dst.IsClosed(), !qtFlag.Load(), dst.Sync())
			return
		}

		i++
		var now = time.Now()
		wr, rd, err := backend.CopyBuffer(dst, src, buf, 100*time.Millisecond, pktPrefix)
		written += wr
		read += rd

		// obey speed limit if set
		if speedLimit > 0 && wr >= speedLimit {
			var wait = time.Until(now.Add(1 * time.Second))
			time.Sleep(wait)
		}

		// stop / skip conditions
		if rd == 0 && err == nil {
			return
		}
		logger.Debug("[%d] T->Q: %v, %v", dst.ID(), wr, err)

		if err != nil {
			if err2, ok := err.(net.Error); ok && err2.Timeout() {
				continue
			}
			logger.Error("[%d] STREAM ERR T->Q: %v", dst.ID(), err)
			closeStreamNow(dst)
			return
		}

		// update speed limit
		if speedLimit > 0 {
			periodWritten += wr
			if periodWritten > speedLimit {
				periodWritten = 0
				<-time.After(time.Until(periodStart.Add(1 * time.Second)))
				periodStart = time.Now()
			}
		}
	}
}

func closeStreamNow(stream backend.QuicBackendStream) {
	stream.AbortWrite(0)
	stream.AbortRead(0)
	_ = stream.Close()
}
