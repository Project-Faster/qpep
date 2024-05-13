package server

import (
	"context"
	"fmt"
	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/backend"
	"github.com/parvit/qpep/flags"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"hash/crc64"
	"io"
	"net"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

const (
	BUFFER_SIZE = 512 * 1024

	DEBUG_DUMP_PACKETS = false
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
	quicListener, err = quicProvider.Listen(context.Background(), address, port, shared.QPepConfig.Certificate, shared.QPepConfig.CertKey,
		shared.QPepConfig.CCAlgorithm, shared.QPepConfig.CCSlowstartAlgo,
		flags.Globals.Trace)

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

func setLinger(c net.Conn) {
	if conn, ok := c.(*net.TCPConn); ok {
		err1 := conn.SetLinger(1)
		logger.OnError(err1, "error on setLinger")
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
		go func(st backend.QuicBackendStream) {
			if st == nil {
				return
			}
			defer func() {
				if err := recover(); err != nil {
					logger.Info("PANIC: %v\n", err)
					debug.PrintStack()
				}
			}()
			tskKey := fmt.Sprintf("QuicStream:%v", st.ID())
			tsk := shared.StartRegion(tskKey)
			defer tsk.End()
			//for i := 0; i < 10; i++ {
			//	connCounter := api.Statistics.GetCounter("", api.TOTAL_CONNECTIONS)
			//	if connCounter >= 16 {
			//		logger.Info("== [%d] Stream Queued (current: %d / max: %d) ==", st.ID(), connCounter, 16)
			//		<-time.After(100 * time.Millisecond)
			//		continue
			//	}
			logger.Info("== [%d] Stream Start ==", st.ID())
			handleQuicStream(st)
			logger.Info("== [%d] Stream End ==", st.ID())
			//	return
			//}
			//logger.Info("== [%d] Session Rejected for too many connections ==", stream.ID())
			//_ = stream.Close()
			//_ = quicSession.Close(1, "Session Rejected for too many connections")
		}(stream)
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

	logger.Debug("[%d] Connection flags : %d %v", quicStream.ID(), qpepHeader.Flags, qpepHeader.Flags&shared.QPEP_LOCALSERVER_DESTINATION != 0)

	// To support the server being behind a private NAT (external gateway address != local listening address)
	// we dial the listening address when the connection is directed at the non-local API server
	destAddress := qpepHeader.DestAddr.String()
	if qpepHeader.Flags&shared.QPEP_LOCALSERVER_DESTINATION != 0 {
		logger.Debug("[%d] Local connection to server", quicStream.ID())
		destAddress = fmt.Sprintf("127.0.0.1:%d", qpepHeader.DestAddr.Port)
	}

	tskKey := fmt.Sprintf("TCP-Dial:%v:%v", quicStream.ID(), destAddress)
	tsk := shared.StartRegion(tskKey)
	logger.Debug("[%d] >> Opening TCP Conn to dest:%s, src:%s\n", quicStream.ID(), destAddress, qpepHeader.SourceAddr)
	dial := &net.Dialer{
		//LocalAddr:     &net.TCPAddr{IP: net.ParseIP(ServerConfiguration.ListenHost)},
		Timeout:       30 * time.Second,
		Deadline:      time.Now().Add(30 * time.Second),
		KeepAlive:     -1,
		DualStack:     false,
		FallbackDelay: -1,
	}
	tcpConn, err := dial.Dial("tcp", destAddress)
	tsk.End()
	if err != nil {
		logger.Error("[%d] Unable to open TCP connection from QPEP quicStream: %s\n", quicStream.ID(), err)
		quicStream.Close()
		return
	}
	logger.Info(">> [%d] Opened TCP Conn %s -> %s\n", quicStream.ID(), qpepHeader.SourceAddr, destAddress)

	trackedAddress := qpepHeader.SourceAddr.IP.String()
	proxyAddress := tcpConn.LocalAddr().String()
	startTime := time.Now()
	tqActiveFlag := atomic.Bool{}
	qtActiveFlag := atomic.Bool{}

	tqActiveFlag.Store(true)
	qtActiveFlag.Store(true)

	//setLinger(tcpConn)

	api.Statistics.IncrementCounter(1.0, api.TOTAL_CONNECTIONS)
	api.Statistics.IncrementCounter(1.0, api.PERF_CONN, trackedAddress)
	defer func() {
		api.Statistics.DecrementCounter(1.0, api.PERF_CONN, trackedAddress)
		api.Statistics.DecrementCounter(1.0, api.TOTAL_CONNECTIONS)
	}()

	ctx, _ := context.WithCancel(context.Background())

	var streamWait sync.WaitGroup
	streamWait.Add(2)

	go handleQuicToTcp(ctx, &streamWait, srcLimit, tcpConn, quicStream, proxyAddress, trackedAddress, &qtActiveFlag, &tqActiveFlag)

	go handleTcpToQuic(ctx, &streamWait, dstLimit, quicStream, tcpConn, trackedAddress, &qtActiveFlag, &tqActiveFlag)

	//we exit (and close the TCP connection) once both streams are done copying or timeout
	logger.Info("== Stream %d Wait ==", quicStream.ID())
	streamWait.Wait()
	logger.Info("== Stream %d (duration: %v) End ==", quicStream.ID(), time.Now().Sub(startTime))

	tcpConn.Close()
	quicStream.Close()

	logger.Info(">> [%d] Closed Conn %s -> %s\n", quicStream.ID(), qpepHeader.SourceAddr, destAddress)
}

func handleQuicToTcp(ctx context.Context, streamWait *sync.WaitGroup, speedLimit int64, dst net.Conn, src backend.QuicBackendStream, proxyAddress, trackedAddress string, qtFlag, tqFlag *atomic.Bool) {

	tempBuffer := make([]byte, BUFFER_SIZE)
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

		api.Statistics.DeleteMappedAddress(proxyAddress)
		tsk.End()
		streamWait.Done()
		qtFlag.Store(false)
		logger.Info("== [%d] Stream Quic->TCP done [wr:%v rd:%d] done ==", src.ID(), written, read)
	}()

	api.Statistics.SetMappedAddress(proxyAddress, trackedAddress)

	pktPrefix := fmt.Sprintf("%v.server.qt", src.ID())
	pktcounter := 0

	i := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if src.IsClosed() || !tqFlag.Load() {
			logger.Error("[%v] LINKED TQ CLOSE (%v %v %v)", src.ID(), src.IsClosed(), !tqFlag.Load(), src.Sync())
			return
		}

		i++
		//if speedLimit == 0 {
		wr, rd, err := copyBuffer(dst, src, tempBuffer, 100*time.Millisecond, 100*time.Millisecond, pktPrefix, &pktcounter)
		pktcounter++
		//} else {
		//	var now = time.Now()
		//	wr, err = io.Copy(dst, io.LimitReader(src, speedLimit))
		//
		//	var wait = time.Until(now.Add(1 * time.Second))
		//	time.Sleep(wait)
		//}

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
			logger.Debug("[%d] END Q->T: %v", src.ID(), err)
			return
		}
	}
}

func handleTcpToQuic(ctx context.Context, streamWait *sync.WaitGroup, speedLimit int64, dst backend.QuicBackendStream, src net.Conn, trackedAddress string, qtFlag, tqFlag *atomic.Bool) {

	tempBuffer := make([]byte, BUFFER_SIZE)
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
		tqFlag.Store(false)
		streamWait.Done()
		logger.Info("== [%d] Stream TCP->Quic [wr:%v rd:%d] done ==", dst.ID(), written, read)
	}()

	pktPrefix := fmt.Sprintf("%v.server.tq", dst.ID())
	pktcounter := 0

	i := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if dst.IsClosed() || !qtFlag.Load() {
			logger.Error("[%v] LINKED TQ CLOSE (%v %v %v)", dst.ID(), dst.IsClosed(), !qtFlag.Load(), dst.Sync())
			return
		}

		i++
		//if speedLimit == 0 {
		wr, rd, err := copyBuffer(dst, src, tempBuffer, 100*time.Millisecond, 100*time.Millisecond, pktPrefix, &pktcounter)
		written += wr
		read += rd

		pktcounter++
		//} else {
		//	var now = time.Now()
		//	wr, err = io.CopyBuffer(dst, io.LimitReader(src, speedLimit), tempBuffer)
		//
		//	var wait = time.Until(now.Add(1 * time.Second))
		//	time.Sleep(wait)
		//}

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
}

func copyBuffer(dst WriterTimeout, src ReaderTimeout, buf []byte, timeoutDst time.Duration, timeoutSrc time.Duration,
	prefix string, counter *int) (written, read int64, err error) {

	//limitSrc := io.LimitReader(src, BUFFER_SIZE)

	src.SetReadDeadline(time.Now().Add(timeoutSrc))

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
