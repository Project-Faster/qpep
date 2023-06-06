package shared

import (
	"bufio"
	"github.com/Project-Faster/quic-go"
	"time"
)

// GetQuicConfiguration method returns the configuration to be used for the
// quic-go library, it is common among the server and client packages
func GetQuicConfiguration() *quic.Config {
	cfg := &quic.Config{
		MaxIncomingStreams:      10240,
		DisablePathMTUDiscovery: false,

		HandshakeIdleTimeout: GetScaledTimeout(10, time.Second),
		//KeepAlivePeriod:      1 * time.Second,

		EnableDatagrams: true,
	}

	// Only used in debug sessions
	//cfg.Tracer = qlog.NewTracer(func(_ logging.Perspective, connID []byte) io.WriteCloser {
	//	filename := fmt.Sprintf("client_%x.qlog", connID)
	//	f, err := os.Create(filename)
	//	if err != nil {
	//		panic(err)
	//	}
	//	logger.Info("Creating qlog file %s.\n", filename)
	//	return &QLogWriter{Writer: bufio.NewWriter(f)}
	//})

	return cfg
}

// QLogWriter struct used by quic-go package to dump debug information
// abount quic connections
type QLogWriter struct {
	*bufio.Writer
}

// Close method flushes the data to internal writer
func (mwc *QLogWriter) Close() error {
	// Noop
	return mwc.Writer.Flush()
}
