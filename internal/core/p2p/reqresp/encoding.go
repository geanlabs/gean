// Package reqresp is the req/resp wire format shared by consensus networks:
// varint-prefixed, snappy-framed payloads, response chunks led by a result
// code, and the idle deadlines that bound a stream.
package reqresp

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"github.com/golang/snappy"
)

const (
	MaxPayloadSize           = 10 * 1024 * 1024
	MaxCompressedPayloadSize = 32 + MaxPayloadSize + MaxPayloadSize/6 + 1024
	MaxErrorMessageSize      = 256
)

func EncodePayload(data []byte) []byte {
	return appendPayload(make([]byte, 0, payloadCap(len(data))), data)
}

// payloadCap bounds the encoding of n bytes: a varint length, the snappy stream
// identifier, and per 64 KiB block a chunk header and at most the block itself,
// since snappy stores a block raw when compressing would not shrink it.
func payloadCap(n int) int {
	return binary.MaxVarintLen64 + 10 + n + 8*(n>>16+1)
}

// snappyWriters reuses framed snappy writers: each holds about 140 KB of
// buffers, which dominated the cost of encoding a chunk.
var snappyWriters = sync.Pool{New: func() any { return snappy.NewBufferedWriter(nil) }}

// appendPayload appends data's length prefix and snappy frames to dst.
func appendPayload(dst, data []byte) []byte {
	buf := bytes.NewBuffer(binary.AppendUvarint(dst, uint64(len(data))))
	w := snappyWriters.Get().(*snappy.Writer)
	w.Reset(buf)
	// Writing to a bytes.Buffer cannot fail.
	_, _ = w.Write(data)
	_ = w.Close()
	w.Reset(nil)
	snappyWriters.Put(w)
	return buf.Bytes()
}

// DecodePayload decodes a request: a single frame that is the whole stream, so
// nothing may follow the declared length.
func DecodePayload(buf []byte) ([]byte, error) {
	r := bytes.NewReader(buf)
	decoded, err := decodeFrame(r, MaxPayloadSize)
	if err != nil {
		return nil, err
	}
	if r.Len() != 0 {
		return nil, fmt.Errorf("%d bytes after the payload", r.Len())
	}
	return decoded, nil
}

const (
	RespSuccess             byte = 0x00
	RespInvalidRequest      byte = 0x01
	RespServerError         byte = 0x02
	RespResourceUnavailable byte = 0x03
)

func EncodeResponse(code byte, data []byte) []byte {
	if code != RespSuccess && len(data) > MaxErrorMessageSize {
		data = data[:MaxErrorMessageSize]
	}
	return appendPayload(append(make([]byte, 0, 1+payloadCap(len(data))), code), data)
}

type byteReader interface {
	io.Reader
	io.ByteScanner
}

func DecodeResponse(r io.Reader, maxLen uint32) (byte, []byte, error) {
	br, ok := r.(byteReader)
	if !ok {
		br = bufio.NewReader(r)
	}

	code, err := br.ReadByte()
	if err != nil {
		return 0, nil, fmt.Errorf("read response code: %w", err)
	}

	decoded, err := decodeFrame(br, maxLen)
	if err != nil {
		return code, nil, err
	}

	if code != RespSuccess && len(decoded) > MaxErrorMessageSize {
		return code, nil, fmt.Errorf("error message %d bytes exceeds MaxErrorMessageSize %d", len(decoded), MaxErrorMessageSize)
	}

	return code, decoded, nil
}

// decodeFrame reads a varint length prefix and the snappy frames carrying the
// payload it declares, of at most maxLen bytes. snappy.Reader reads one frame
// at a time, so stopping at the declared length leaves whatever follows, such
// as the next response chunk, in br. A frame decoding past the declared length
// is refused rather than cut short.
func decodeFrame(br byteReader, maxLen uint32) ([]byte, error) {
	declaredLen, err := decodeVarintFrom(br)
	if err != nil {
		return nil, fmt.Errorf("decode payload length: %w", err)
	}
	if declaredLen > maxLen {
		return nil, fmt.Errorf("payload length %d exceeds max %d", declaredLen, maxLen)
	}
	if declaredLen == 0 {
		return []byte{}, skipStreamIdentifier(br)
	}

	sr := snappy.NewReader(io.LimitReader(br, int64(maxFramedLen(declaredLen))))
	// Grow with the bytes that arrive rather than trusting the declared
	// length up front. A read returns at most one frame, which the extra byte
	// of room exposes when it decodes past the declared length.
	buf := make([]byte, maxFrameDecodedLen)
	want := int(declaredLen)
	var decoded []byte
	for len(decoded) < want {
		n, err := sr.Read(buf[:min(len(buf), want-len(decoded)+1)])
		if err != nil {
			return nil, fmt.Errorf("decode payload: %w", err)
		}
		if len(decoded)+n > want {
			return nil, fmt.Errorf("length mismatch: declared %d, got at least %d", declaredLen, len(decoded)+n)
		}
		decoded = append(decoded, buf[:n]...)
	}
	return decoded, nil
}

// Snappy framing: https://github.com/google/snappy/blob/main/framing_format.txt
const (
	maxFrameDecodedLen = 65536
	streamIdentifier   = "\xff\x06\x00\x00sNaPpY"
)

// maxFramedLen bounds the framed bytes a payload of n bytes can take: the
// stream identifier, then a frame per started 64 KiB, each a 4-byte header, a
// 4-byte checksum and at most snappy's worst-case encoding of 64 KiB.
func maxFramedLen(n uint32) int {
	frames := int(n)/maxFrameDecodedLen + 1
	return len(streamIdentifier) + frames*(4+4+snappy.MaxEncodedLen(maxFrameDecodedLen))
}

// skipStreamIdentifier consumes the stream identifier that may follow a zero
// length: an empty payload is written either as the length alone or with the
// identifier. A frame starts 0xff where a response code or the end of the
// stream follows otherwise, so one byte tells them apart.
func skipStreamIdentifier(br byteReader) error {
	first, err := br.ReadByte()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	if first != streamIdentifier[0] {
		return br.UnreadByte()
	}
	rest := make([]byte, len(streamIdentifier)-1)
	if _, err := io.ReadFull(br, rest); err != nil || string(rest) != streamIdentifier[1:] {
		return fmt.Errorf("decode payload: malformed stream identifier after a zero length")
	}
	return nil
}

func decodeVarintFrom(r io.ByteReader) (uint32, error) {
	val, err := binary.ReadUvarint(r)
	if err != nil {
		return 0, err
	}
	if val > uint64(MaxPayloadSize) {
		return 0, fmt.Errorf("varint value %d exceeds max payload", val)
	}
	return uint32(val), nil
}
