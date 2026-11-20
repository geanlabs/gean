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

	"github.com/golang/snappy"
)

const (
	MaxPayloadSize           = 10 * 1024 * 1024
	MaxCompressedPayloadSize = 32 + MaxPayloadSize + MaxPayloadSize/6 + 1024
	MaxErrorMessageSize      = 256
)

func EncodeVarint(value uint32) []byte {
	buf := make([]byte, binary.MaxVarintLen32)
	n := binary.PutUvarint(buf, uint64(value))
	return buf[:n]
}

func EncodePayload(data []byte) []byte {
	var buf bytes.Buffer
	w := snappy.NewBufferedWriter(&buf)
	w.Write(data)
	w.Close()
	framed := buf.Bytes()

	varint := EncodeVarint(uint32(len(data)))
	result := make([]byte, len(varint)+len(framed))
	copy(result, varint)
	copy(result[len(varint):], framed)
	return result
}

// DecodePayload decodes a request: a single frame that is the whole stream, so
// nothing may follow the declared length.
func DecodePayload(buf []byte) ([]byte, error) {
	decoded, sr, err := decodeFrame(bytes.NewReader(buf), MaxPayloadSize)
	if err != nil {
		return nil, err
	}
	if err := expectFrameEnd(sr, len(decoded)); err != nil {
		return nil, err
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
	payload := EncodePayload(data)
	result := make([]byte, 1+len(payload))
	result[0] = code
	copy(result[1:], payload)
	return result
}

type byteReader interface {
	io.Reader
	io.ByteReader
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

	decoded, sr, err := decodeFrame(br, maxLen)
	if err != nil {
		return code, nil, err
	}
	// Reading past a non-empty payload would consume the next chunk, but an
	// empty one must still be an empty snappy stream.
	if len(decoded) == 0 {
		if err := expectFrameEnd(sr, 0); err != nil {
			return code, nil, err
		}
	}

	if code != RespSuccess && len(decoded) > MaxErrorMessageSize {
		return code, nil, fmt.Errorf("error message %d bytes exceeds MaxErrorMessageSize %d", len(decoded), MaxErrorMessageSize)
	}

	return code, decoded, nil
}

// decodeFrame reads a varint length prefix and the snappy-framed payload it
// declares, of at most maxLen bytes. It returns the snappy reader for callers
// that check what follows.
func decodeFrame(br byteReader, maxLen uint32) ([]byte, io.Reader, error) {
	declaredLen, err := decodeVarintFrom(br)
	if err != nil {
		return nil, nil, fmt.Errorf("decode payload length: %w", err)
	}
	if declaredLen > maxLen {
		return nil, nil, fmt.Errorf("payload length %d exceeds max %d", declaredLen, maxLen)
	}

	sr := snappy.NewReader(br)
	// Grow with the bytes that arrive rather than trusting the declared
	// length up front.
	decoded, err := io.ReadAll(io.LimitReader(sr, int64(declaredLen)))
	if err != nil {
		return nil, nil, fmt.Errorf("decode payload: %w", err)
	}
	if uint32(len(decoded)) != declaredLen {
		return nil, nil, fmt.Errorf("length mismatch: declared %d, got %d", declaredLen, len(decoded))
	}
	return decoded, sr, nil
}

func expectFrameEnd(sr io.Reader, declaredLen int) error {
	var scratch [1]byte
	if n, err := sr.Read(scratch[:]); err != io.EOF {
		if err != nil {
			return fmt.Errorf("decode payload: %w", err)
		}
		return fmt.Errorf("length mismatch: declared %d, got at least %d", declaredLen, declaredLen+n)
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
