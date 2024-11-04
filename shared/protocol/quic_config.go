package protocol

import (
	"bufio"
)

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
