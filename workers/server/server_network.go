package server

import (
	"context"
	"fmt"
	"github.com/parvit/qpep/api"
	"github.com/parvit/qpep/backend"
	"github.com/parvit/qpep/logger"
	"github.com/parvit/qpep/shared"
	"hash/crc64"
	"io"
	"net"
	"os"
	"runtime/debug"
	"sync"
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
			tskKey := fmt.Sprintf("QuicStream:%v", stream.ID())
			tsk := shared.StartRegion(tskKey)
			defer tsk.End()
			//for i := 0; i < 10; i++ {
			//	connCounter := api.Statistics.GetCounter("", api.TOTAL_CONNECTIONS)
			//	if connCounter >= 16 {
			//		logger.Info("== [%d] Stream Queued (current: %d / max: %d) ==", stream.ID(), connCounter, 16)
			//		<-time.After(100 * time.Millisecond)
			//		continue
			//	}
			logger.Info("== [%d] Stream Start ==", stream.ID())
			handleQuicStream(stream)
			logger.Info("== [%d] Stream End ==", stream.ID())
			//	return
			//}
			//logger.Info("== [%d] Session Rejected for too many connections ==", stream.ID())
			//_ = stream.Close()
			//_ = quicSession.Close(1, "Session Rejected for too many connections")
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

	tskKey := fmt.Sprintf("TCP-Dial:%v:%v", quicStream.ID(), destAddress)
	tsk := shared.StartRegion(tskKey)
	logger.Debug("[%d] >> Opening TCP Conn to dest:%s, src:%s\n", quicStream.ID(), destAddress, qpepHeader.SourceAddr)
	dial := &net.Dialer{
		LocalAddr:     &net.TCPAddr{IP: net.ParseIP(ServerConfiguration.ListenHost)},
		Timeout:       10 * time.Second,
		KeepAlive:     5 * time.Second,
		DualStack:     true,
		FallbackDelay: 10 * time.Millisecond,
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

	//we exit (and close the TCP connection) once both streams are done copying or timeout
	logger.Info("== Stream %d Wait ==", quicStream.ID())
	streamWait.Wait()
	logger.Info("== Stream %d WaitEnd ==", quicStream.ID())

	tcpConn.Close()
	quicStream.Close()
}

func handleQuicToTcp(ctx context.Context, streamWait *sync.WaitGroup, speedLimit int64,
	dst net.Conn, src backend.QuicBackendStream, proxyAddress, trackedAddress string) {

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
		logger.Info("== [%d] Stream Quic->TCP done ==", src.ID())
	}()

	api.Statistics.SetMappedAddress(proxyAddress, trackedAddress)

	//setLinger(dst)

	pktPrefix := fmt.Sprintf("%v.server.qt", src.ID())
	pktcounter := 0

	var tempBuffer = make([]byte, BUFFER_SIZE)

	var err error = nil
	i := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		//logger.Info("[%d] Q->T: %v: %v", src.ID(), activityFlag, *activityFlag)

		i++
		//if speedLimit == 0 {
		_, err = copyBuffer(dst, src, tempBuffer, pktPrefix, &pktcounter)
		pktcounter++
		//} else {
		//	var now = time.Now()
		//	wr, err = io.Copy(dst, io.LimitReader(src, speedLimit))
		//
		//	var wait = time.Until(now.Add(1 * time.Second))
		//	time.Sleep(wait)
		//}

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

	tskKey := fmt.Sprintf("Tcp->Quic:%v", dst.ID())
	tsk := shared.StartRegion(tskKey)
	defer func() {
		if err := recover(); err != nil {
			logger.Error("ERR: %v", err)
			debug.PrintStack()
		}
		tsk.End()
		streamWait.Done()
		logger.Info("== [%d] Stream TCP->Quic done ==", dst.ID())
	}()

	//setLinger(src)

	var tempBuffer = make([]byte, BUFFER_SIZE)

	pktPrefix := fmt.Sprintf("%v.server.tq", dst.ID())
	pktcounter := 0

	var err error = nil
	i := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Millisecond):
		}

		//logger.Info("[%d] T->Q: %v: %v", dst.ID(), activityFlag, *activityFlag)

		i++
		//if speedLimit == 0 {
		_, err = copyBuffer(dst, src, tempBuffer, pktPrefix, &pktcounter)
		pktcounter++
		//} else {
		//	var now = time.Now()
		//	wr, err = io.CopyBuffer(dst, io.LimitReader(src, speedLimit), tempBuffer)
		//
		//	var wait = time.Until(now.Add(1 * time.Second))
		//	time.Sleep(wait)
		//}

		if err != nil {
			if err, ok := err.(net.Error); ok && err.Timeout() {
				continue
			}
			logger.Info("[%d] END T->Q: %v", dst.ID(), err)
			return
		}
	}
}

func copyBuffer(dst WriterTimeout, src ReaderTimeout, buf []byte, prefix string, counter *int) (written int64, err error) {
	limitSrc := io.LimitReader(src, BUFFER_SIZE)
	lastActivity := time.Now()

	for {
		src.SetReadDeadline(time.Now().Add(10 * time.Second))

		nr, er := limitSrc.Read(buf)

		if nr > 0 {
			lastActivity = time.Now()

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
				logger.Info("[%d][%s] rd: %d (%v)", *counter, dump.Name(), nr, crc64.Checksum(buf[0:nr], crc64.MakeTable(crc64.ISO)))
			} else {
				logger.Debug("[%d] rd: %d", *counter, nr)
			}

			dst.SetWriteDeadline(time.Now().Add(10 * time.Second))

			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = io.ErrUnexpectedEOF
				}
			}

			if DEBUG_DUMP_PACKETS {
				dump, derr := os.Create(fmt.Sprintf("%s.%s.%d-wr.bin", prefix, shared.QPepConfig.Backend, *counter))
				if derr != nil {
					panic(derr)
				}
				dump.Write(buf[0:nw])
				go func() {
					dump.Sync()
					dump.Close()
				}()
				logger.Info("[%d][%s] wr: %d (%v)", *counter, dump.Name(), nw, crc64.Checksum(buf[0:nw], crc64.MakeTable(crc64.ISO)))
			} else {
				logger.Debug("[%d] wr: %d", *counter, nw)
			}
			*counter = *counter + 1

			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		} else {
			logger.Debug("[%d][%s] w,r: %d,%v **", *counter, prefix, 0, er)
		}

		if time.Now().Sub(lastActivity) > 1*time.Second {
			logger.Error("[%s] ACTIVITY TIMEOUT", prefix)
			return written, io.ErrNoProgress
		}

		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}
