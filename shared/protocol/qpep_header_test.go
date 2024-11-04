package protocol

import (
	"bytes"
	"github.com/Project-Faster/qpep/shared/errors"
	"github.com/stretchr/testify/assert"
	"io"
	"net"
	"testing"
	"time"
)

// Header -> Bytes
func TestQPepHeader_ToBytes(t *testing.T) {
	srcAddr := &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 9443,
	}
	dstAddr := &net.TCPAddr{
		IP:   net.ParseIP("192.168.1.100"),
		Port: 445,
	}

	header := &QPepHeader{
		SourceAddr: srcAddr,
		DestAddr:   dstAddr,
	}
	var expectValue = []byte{
		0x04,                   // type src
		0x04,                   // type dst
		0x7f, 0x00, 0x00, 0x01, // src ip
		0xe3, 0x24, // src port
		0xc0, 0xa8, 0x01, 0x64, // dst ip
		0xbd, 0x01, // dst port
		0x00, 0x00, // flags
	}

	var outValue = header.ToBytes()
	assertArrayEquals(t, expectValue, outValue)
}

func TestQPepHeader_ToBytesV6(t *testing.T) {
	srcAddr := &net.TCPAddr{
		IP:   net.ParseIP("::1"),
		Port: 9443,
	}
	dstAddr := &net.TCPAddr{
		IP:   net.ParseIP("::1"),
		Port: 445,
	}

	header := &QPepHeader{
		SourceAddr: srcAddr,
		DestAddr:   dstAddr,
	}
	var expectValue = []byte{
		0x06, // type src
		0x06, // type dst
		// src ip
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		// src port
		0xe3, 0x24,
		// dst ip
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		// dst port
		0xbd, 0x01,
		0x00, 0x00, // flags
	}

	var outValue = header.ToBytes()
	assertArrayEquals(t, expectValue, outValue)
}

func TestQPepHeader_ToBytesV4V6(t *testing.T) {
	srcAddr := &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 9443,
	}
	dstAddr := &net.TCPAddr{
		IP:   net.ParseIP("::1"),
		Port: 445,
	}

	header := &QPepHeader{
		SourceAddr: srcAddr,
		DestAddr:   dstAddr,
	}
	var expectValue = []byte{
		0x04,                   // type src
		0x06,                   // type dst
		0x7f, 0x00, 0x00, 0x01, // src ip
		0xe3, 0x24, // src port
		// dst ip
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		// dst port
		0xbd, 0x01,
		0x00, 0x00, // flags
	}

	var outValue = header.ToBytes()
	assertArrayEquals(t, expectValue, outValue)
}

func TestQPepHeader_ToBytesV6V4(t *testing.T) {
	srcAddr := &net.TCPAddr{
		IP:   net.ParseIP("::1"),
		Port: 9443,
	}
	dstAddr := &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 445,
	}

	header := &QPepHeader{
		SourceAddr: srcAddr,
		DestAddr:   dstAddr,
	}
	var expectValue = []byte{
		0x06, // type src
		0x04, // type dst
		// src ip
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		// src port
		0xe3, 0x24,
		// dst ip
		0x7f, 0x00, 0x00, 0x01, // src ip
		// dst port
		0xbd, 0x01,
		0x00, 0x00, // flags
	}

	var outValue = header.ToBytes()
	assertArrayEquals(t, expectValue, outValue)
}

func TestQPepHeader_ToBytes_NilSrcDst(t *testing.T) {
	header := &QPepHeader{}

	var outValue = header.ToBytes()
	assert.Nil(t, outValue)
}

func TestQPepHeader_ToBytes_NilSrc(t *testing.T) {
	dstAddr := &net.TCPAddr{
		IP:   net.ParseIP("192.168.1.100"),
		Port: 445,
	}
	header := &QPepHeader{
		DestAddr: dstAddr,
	}

	var outValue = header.ToBytes()
	assert.Nil(t, outValue)
}

func TestQPepHeader_ToBytes_NilDst(t *testing.T) {
	srcAddr := &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 9443,
	}
	header := &QPepHeader{
		SourceAddr: srcAddr,
	}

	var outValue = header.ToBytes()
	assert.Nil(t, outValue)
}

// Bytes -> Header
func TestQPepHeader_FromBytes(t *testing.T) {
	srcAddr := &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 9443,
	}
	dstAddr := &net.TCPAddr{
		IP:   net.ParseIP("192.168.1.100"),
		Port: 445,
	}

	header := &QPepHeader{
		SourceAddr: srcAddr,
		DestAddr:   dstAddr,
		Flags:      QPEP_LOCALSERVER_DESTINATION,
	}
	var testValue = []byte{
		0x04,                   // type src
		0x04,                   // type dst
		0x7f, 0x00, 0x00, 0x01, // src ip
		0xe3, 0x24, // src port
		0xc0, 0xa8, 0x01, 0x64, // dst ip
		0xbd, 0x01, // dst port
		0x01, 0x00, // flags
	}

	buf := &TimeoutReader{
		Reader: bytes.NewReader(testValue),
	}

	decodeHeader, err := QPepHeaderFromBytes(buf)
	assert.Nil(t, err)

	assertHeadersEquals(t, header, decodeHeader)
}

func TestQPepHeader_FromBytesV6(t *testing.T) {
	srcAddr := &net.TCPAddr{
		IP:   net.ParseIP("::1"),
		Port: 9443,
	}
	dstAddr := &net.TCPAddr{
		IP:   net.ParseIP("::1"),
		Port: 445,
	}

	header := &QPepHeader{
		SourceAddr: srcAddr,
		DestAddr:   dstAddr,
	}
	var testValue = []byte{
		0x06, // type src
		0x06, // type dst
		// src ip
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		// src port
		0xe3, 0x24,
		// dst ip
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		// dst port
		0xbd, 0x01,
		0x00, 0x00, // flags
	}

	buf := &TimeoutReader{
		Reader: bytes.NewReader(testValue),
	}

	decodeHeader, err := QPepHeaderFromBytes(buf)
	assert.Nil(t, err)

	assertHeadersEquals(t, header, decodeHeader)
}

func TestQPepHeader_FromBytesV4V6(t *testing.T) {
	srcAddr := &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 9443,
	}
	dstAddr := &net.TCPAddr{
		IP:   net.ParseIP("::1"),
		Port: 445,
	}

	header := &QPepHeader{
		SourceAddr: srcAddr,
		DestAddr:   dstAddr,
	}
	var testValue = []byte{
		0x04,                   // type src
		0x06,                   // type dst
		0x7f, 0x00, 0x00, 0x01, // src ip
		0xe3, 0x24, // src port
		// dst ip
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		// dst port
		0xbd, 0x01,
		0x00, 0x00, // flags
	}

	buf := &TimeoutReader{
		Reader: bytes.NewReader(testValue),
	}

	decodeHeader, err := QPepHeaderFromBytes(buf)
	assert.Nil(t, err)

	assertHeadersEquals(t, header, decodeHeader)
}

func TestQPepHeader_FromBytesV6V4(t *testing.T) {
	srcAddr := &net.TCPAddr{
		IP:   net.ParseIP("::1"),
		Port: 9443,
	}
	dstAddr := &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 445,
	}

	header := &QPepHeader{
		SourceAddr: srcAddr,
		DestAddr:   dstAddr,
	}
	var testValue = []byte{
		0x06, // type src
		0x04, // type dst
		// src ip
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		// src port
		0xe3, 0x24,
		// dst ip
		0x7f, 0x00, 0x00, 0x01, // src ip
		// dst port
		0xbd, 0x01,
		0x00, 0x00, // flags
	}

	buf := &TimeoutReader{
		Reader: bytes.NewReader(testValue),
	}

	decodeHeader, err := QPepHeaderFromBytes(buf)
	assert.Nil(t, err)

	assertHeadersEquals(t, header, decodeHeader)
}

func TestQPepHeader_FromBytes_NilSrcDst(t *testing.T) {
	var testValue = []byte{
		0x00, // type src
		0x00, // type dst
	}

	buf := &TimeoutReader{
		Reader: bytes.NewReader(testValue),
	}

	decodeHeader, err := QPepHeaderFromBytes(buf)
	assert.Nil(t, decodeHeader)
	assert.Equal(t, errors.ErrInvalidHeaderAddressType, err)
}

func TestQPepHeader_FromBytes_NilSrc(t *testing.T) {
	var testValue = []byte{
		0x00, // type src
		0x04, // type dst
		// src ip
		// dst ip
		0x7f, 0x00, 0x00, 0x01, // src ip
		// dst port
		0xbd, 0x01,
		0x00, 0x00, // flags
	}

	buf := &TimeoutReader{
		Reader: bytes.NewReader(testValue),
	}

	decodeHeader, err := QPepHeaderFromBytes(buf)
	assert.Nil(t, decodeHeader)
	assert.Equal(t, errors.ErrInvalidHeaderAddressType, err)
}

func TestQPepHeader_FromBytes_NilDst(t *testing.T) {
	var testValue = []byte{
		0x06, // type src
		0x00, // type dst
		// src ip
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		// src port
		0xe3, 0x24,
		0x00, 0x00, // flags
	}

	buf := &TimeoutReader{
		Reader: bytes.NewReader(testValue),
	}
	decodeHeader, err := QPepHeaderFromBytes(buf)
	assert.Nil(t, decodeHeader)
	assert.Equal(t, errors.ErrInvalidHeaderAddressType, err)
}

// Malformed packets read
func TestQPepHeader_FromBytes_MalformedPreamble(t *testing.T) {
	var testValue_len0 = make([]byte, 0)

	buf := &TimeoutReader{
		Reader: bytes.NewReader(testValue_len0),
	}
	decodeHeader, err := QPepHeaderFromBytes(buf)
	assert.Nil(t, decodeHeader)
	assert.Equal(t, errors.ErrInvalidHeader, err)

	var testValue_len1 = []byte{0x04}
	buf = &TimeoutReader{
		Reader: bytes.NewReader(testValue_len1),
	}
	decodeHeader, err = QPepHeaderFromBytes(buf)
	assert.Nil(t, decodeHeader)
	assert.Equal(t, errors.ErrInvalidHeader, err)
}

func TestQPepHeader_FromBytes_MalformedDataTruncated(t *testing.T) {
	var testValue = []byte{
		0x04,                   // type src
		0x04,                   // type dst
		0x7f, 0x00, 0x00, 0x01, // src ip
		0xe3, 0x24, // src port
		0xc0, 0xa8, 0x01, 0x64, // dst ip
		0xbd, // missing last 0x01 byte
	}

	buf := &TimeoutReader{
		Reader: bytes.NewReader(testValue),
	}
	decodeHeader, err := QPepHeaderFromBytes(buf)
	assert.Nil(t, decodeHeader)
	assert.Equal(t, errors.ErrInvalidHeaderDataLength, err)
}

// Base conversions
func TestIpToBytes(t *testing.T) {
	assertArrayEquals(t, ipToBytes(nil, 0x00), nil)

	assertArrayEquals(t, ipToBytes(net.ParseIP("127.0.0.1"), 0x04),
		[]byte{0x7f, 0x00, 0x00, 0x01})

	assertArrayEquals(t, ipToBytes(net.ParseIP("::1"), 0x04), nil)

	assertArrayEquals(t, ipToBytes(net.ParseIP("::1"), 0x06),
		[]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01})

	assertArrayEquals(t, ipToBytes(net.ParseIP("127.0.0.1"), 0x06),
		[]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0xff, 0xff, 0x7f, 0x00, 0x00, 0x01})
}

func TestPortToBytes(t *testing.T) {
	assertArrayEquals(t, []byte{0x00, 0x00}, portToBytes(0))

	assertArrayEquals(t, []byte{0xff, 0x7f}, portToBytes(32767))

	assertArrayEquals(t, []byte{0xff, 0xff}, portToBytes(65535))

	assertArrayEquals(t, nil, portToBytes(-1000))

	assertArrayEquals(t, nil, portToBytes(99999))
}

// Utils
func assertArrayEquals(t *testing.T, vec_a, vec_b []byte) {
	assert.Equal(t, len(vec_a), len(vec_b))
	if t.Failed() {
		t.Logf("a: %v, b: %v\n", vec_a, vec_b)
		return
	}

	for i := 0; i < len(vec_a); i++ {
		assert.Equal(t, vec_a[i], vec_b[i])
		if t.Failed() {
			t.Logf("a: %v, b: %v\n", vec_a, vec_b)
			return
		}
	}
}

func assertHeadersEquals(t *testing.T, header *QPepHeader, header2 *QPepHeader) {
	if header.SourceAddr != nil && header2.SourceAddr != nil {
		assert.Equal(t, header.SourceAddr.String(), header2.SourceAddr.String())
	} else if header.SourceAddr != nil || header2.SourceAddr != nil {
		assert.Fail(t, "Source addresses must be both nil or both the same value")
		return
	}

	if header.DestAddr != nil && header2.DestAddr != nil {
		assert.Equal(t, header.DestAddr.String(), header2.DestAddr.String())
	} else if header.DestAddr != nil || header2.DestAddr != nil {
		assert.Fail(t, "Destination addresses must be both nil or both the same value")
		return
	}

	assert.Equal(t, header.Flags, header2.Flags, "Flags values are not equal")
}

type TimeoutReader struct {
	io.Reader

	timeout time.Time
}

func (r *TimeoutReader) SetReadDeadline(tDuration time.Time) error {
	r.timeout = tDuration
	return nil
}

var _ readTimeoutReader = &TimeoutReader{}
