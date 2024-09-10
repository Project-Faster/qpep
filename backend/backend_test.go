package backend

import (
	"bou.ke/monkey"
	"bytes"
	"fmt"
	"github.com/parvit/qpep/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"io"
	"net"
	"os"
	"testing"
	"time"
)

func TestBackendSuite(t *testing.T) {
	var q BackendSuite
	suite.Run(t, &q)
}

type BackendSuite struct {
	suite.Suite
}

func (s *BackendSuite) BeforeTest(_, testName string) {
}

func (s *BackendSuite) AfterTest(_, testName string) {
	monkey.UnpatchAll()
}

func (s *BackendSuite) TestGenerateTLSConfig() {
	config := GenerateTLSConfig("cert.pem", "key.pem")
	assert.NotNil(s.T(), config)
}

func (s *BackendSuite) TestCopyBufferShortCopy() {
	src := newTestReaderWriter()
	dst := newTestReaderWriter()

	const testString = "hello world\n"
	src.readBuffer.WriteString(testString)

	buffer := make([]byte, 32*1024)
	wr, rd, err := CopyBuffer(dst, src, buffer, 1*time.Second, "test-")

	assert.Nil(s.T(), err)
	assert.Equal(s.T(), int64(len(testString)), rd)
	assert.Equal(s.T(), int64(len(testString)), wr)
	assert.Equal(s.T(), testString, string(buffer[:rd]))
}

func (s *BackendSuite) TestCopyBufferCloseSrc() {
	src := newTestReaderWriter()
	dst := newTestReaderWriter()

	const testString = "hello world\n"
	src.readBuffer.WriteString(testString)
	src.Close()

	buffer := make([]byte, 32*1024)
	wr, rd, err := CopyBuffer(dst, src, buffer, 1*time.Second, "test-")

	assert.Nil(s.T(), err)
	assert.Equal(s.T(), int64(0), rd)
	assert.Equal(s.T(), int64(0), wr)
}

func (s *BackendSuite) TestCopyBufferCloseDst() {
	src := newTestReaderWriter()
	dst := newTestReaderWriter()

	const testString = "hello world\n"
	src.readBuffer.WriteString(testString)
	dst.Close()

	buffer := make([]byte, 32*1024)
	wr, rd, err := CopyBuffer(dst, src, buffer, 1*time.Second, "test-")

	assert.Nil(s.T(), err)
	assert.Equal(s.T(), int64(len(testString)), rd)
	assert.Equal(s.T(), int64(0), wr)
}

func (s *BackendSuite) TestCopyBufferTimeoutSrc() {
	src := newTestReaderWriter()
	dst := newTestReaderWriter()

	const testString = "hello world\n"
	src.readBuffer.WriteString(testString)
	src.readDelay = 200 * time.Millisecond
	src.readDeadline = time.Now().Add(100 * time.Millisecond)

	buffer := make([]byte, 32*1024)
	wr, rd, err := CopyBuffer(dst, src, buffer, 1*time.Second, "test-")

	assert.Nil(s.T(), err)
	assert.Equal(s.T(), int64(len(testString)/2), rd)
	assert.Equal(s.T(), int64(len(testString)/2), wr)
}

func (s *BackendSuite) TestCopyBufferTimeoutDst() {
	src := newTestReaderWriter()
	dst := newTestReaderWriter()

	const testString = "hello world\n"
	src.readBuffer.WriteString(testString)

	dst.writeDelay = 200 * time.Millisecond
	dst.writeDeadline = time.Now().Add(100 * time.Millisecond)

	buffer := make([]byte, 32*1024)
	wr, rd, err := CopyBuffer(dst, src, buffer, 1*time.Second, "test-")

	assert.Nil(s.T(), err)
	assert.Equal(s.T(), int64(len(testString)), rd)
	assert.Equal(s.T(), int64(len(testString)), wr)
}

func (s *BackendSuite) TestCopyBufferWriteDstNetworkError() {
	src := newTestReaderWriter()
	dst := newTestReaderWriter()

	const testString = "hello world\n"
	src.readBuffer.WriteString(testString)

	dst.writeError = networkError

	buffer := make([]byte, 32*1024)
	wr, rd, err := CopyBuffer(dst, src, buffer, 1*time.Second, "test-")

	assert.NotNil(s.T(), err)
	assert.Equal(s.T(), networkError, err)
	assert.Equal(s.T(), int64(len(testString)), rd)
	assert.Equal(s.T(), int64(0), wr)
}

func (s *BackendSuite) TestCopyBufferWriteDstEOF() {
	src := newTestReaderWriter()
	dst := newTestReaderWriter()

	const testString = "hello world\n"
	src.readBuffer.WriteString(testString)

	dst.writeError = io.EOF // actually produces 0,nil

	buffer := make([]byte, 32*1024)
	wr, rd, err := CopyBuffer(dst, src, buffer, 1*time.Second, "test-")

	assert.NotNil(s.T(), err)
	assert.Equal(s.T(), io.ErrUnexpectedEOF, err)
	assert.Equal(s.T(), int64(len(testString)), rd)
	assert.Equal(s.T(), int64(0), wr)
}

func (s *BackendSuite) TestDumpPacket() {
	shared.DEBUG_DUMP_PACKETS = true
	defer func() {
		shared.DEBUG_DUMP_PACKETS = false
	}()

	rdFile := fmt.Sprintf("%s.%d-%s.bin", "test", localPacketCounter, "rd")
	wrFile := fmt.Sprintf("%s.%d-%s.bin", "test", localPacketCounter, "wr")
	os.Remove(rdFile)
	os.Remove(wrFile)

	dumpPacket("rd", "test", []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, 4)
	dumpPacket("wr", "test", []byte{0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa}, 4)

	dataRd, errRd := os.ReadFile(rdFile)
	dataWr, errWr := os.ReadFile(wrFile)

	assert.Nil(s.T(), errRd)
	assert.Nil(s.T(), errWr)

	assert.Len(s.T(), dataRd, 4)
	assert.Len(s.T(), dataWr, 4)

	for i := 0; i < 4; i++ {
		assert.Equal(s.T(), uint8(0xff), dataRd[i])
	}
	for i := 0; i < 4; i++ {
		assert.Equal(s.T(), uint8(0xaa), dataWr[i])
	}
}

// ----- Support ----- //

func newTestReaderWriter() *testReadWriterTimeout {
	return &testReadWriterTimeout{
		closed:     false,
		closeError: nil,

		writeDeadline: zeroTime,
		writeDelay:    zeroDelay,
		writeBuffer:   bytes.NewBuffer(make([]byte, 0, 32*1024)),

		readDeadline: zeroTime,
		readDelay:    zeroDelay,
		readBuffer:   bytes.NewBuffer(make([]byte, 0, 32*1024)),
	}
}

var zeroTime = time.Time{}
var zeroDelay = time.Duration(0)

type timeoutErrorType struct{}

func (e *timeoutErrorType) Error() string {
	return "stream timed-out"
}
func (e *timeoutErrorType) Timeout() bool {
	return true
}
func (e *timeoutErrorType) Temporary() bool {
	return true
}

var timeoutError = &timeoutErrorType{}
var _ net.Error = timeoutError

type networkErrorType struct{}

func (e *networkErrorType) Error() string {
	return "network error"
}
func (e *networkErrorType) Timeout() bool {
	return false
}
func (e *networkErrorType) Temporary() bool {
	return true
}

var networkError = &networkErrorType{}
var _ net.Error = networkError

type testReadWriterTimeout struct {
	closed     bool
	closeError error

	// writer
	writeDeadline time.Time
	writeDelay    time.Duration
	writeError    error
	writeBuffer   *bytes.Buffer

	// reader
	readDeadline time.Time
	readDelay    time.Duration
	readError    error
	readBuffer   *bytes.Buffer
}

func (t *testReadWriterTimeout) Write(p []byte) (n int, err error) {
	if t.closed {
		return 0, io.EOF
	}
	if t.writeError != nil {
		if t.writeError == io.EOF {
			return 0, nil // special case
		}
		return 0, t.writeError
	}

	if t.writeDelay != zeroDelay {
		<-time.After(t.writeDelay)
	}
	if !t.writeDeadline.IsZero() && time.Now().After(t.writeDeadline) {
		t.writeBuffer.Write(p[:len(p)/2])
		t.writeDeadline = zeroTime
		return len(p) / 2, timeoutError
	}

	return t.writeBuffer.Write(p)
}

func (t *testReadWriterTimeout) SetWriteDeadline(time.Time) error {
	return nil
}

func (t *testReadWriterTimeout) Close() error {
	t.closed = true
	return t.closeError
}

func (t *testReadWriterTimeout) Read(p []byte) (n int, err error) {
	if t.closed {
		return 0, io.EOF
	}
	if t.readError != nil {
		return 0, t.readError
	}

	if t.readDelay != zeroDelay {
		<-time.After(t.readDelay)
	}
	if !t.readDeadline.IsZero() && time.Now().After(t.readDeadline) {
		read := t.readBuffer.Len() / 2
		t.readBuffer.Read(p[:read])
		t.readDeadline = zeroTime
		return read, timeoutError
	}

	return t.readBuffer.Read(p)
}

func (t *testReadWriterTimeout) SetReadDeadline(time.Time) error {
	return nil
}

var _ WriterTimeout = &testReadWriterTimeout{}

var _ ReaderTimeout = &testReadWriterTimeout{}
