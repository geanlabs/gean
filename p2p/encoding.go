package p2p

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/golang/snappy"
)

// Max payload sizes rs L6-9.
const (
	MaxPayloadSize           = 10 * 1024 * 1024 // 10 MiB uncompressed
	MaxCompressedPayloadSize = 32 + MaxPayloadSize + MaxPayloadSize/6 + 1024 // ~12 MiB
)

// --- Gossipsub encoding: raw snappy ---

// SnappyRawEncode compresses data using raw snappy (no framing).
func SnappyRawEncode(data []byte) []byte {
	return snappy.Encode(nil, data)
}

// SnappyRawDecode decompresses raw snappy data.
func SnappyRawDecode(data []byte) ([]byte, error) {
	decodedLen, err := snappy.DecodedLen(data)
	if err != nil {
		return nil, fmt.Errorf("snappy decoded len: %w", err)
	}
	if decodedLen > MaxPayloadSize {
		return nil, fmt.Errorf("snappy decoded len %d exceeds max %d", decodedLen, MaxPayloadSize)
	}
	return snappy.Decode(nil, data)
}

// --- Req/Resp encoding: snappy framed + varint ---

// EncodeVarint encodes a uint32 as LEB128 varint.
func EncodeVarint(value uint32) []byte {
	buf := make([]byte, binary.MaxVarintLen32)
	n := binary.PutUvarint(buf, uint64(value))
	return buf[:n]
}

// DecodeVarint reads a LEB128 varint from a byte slice.
// Returns the value and remaining bytes.
func DecodeVarint(buf []byte) (uint32, []byte, error) {
	val, n := binary.Uvarint(buf)
	if n <= 0 {
		return 0, nil, fmt.Errorf("invalid varint")
	}
	if val > uint64(MaxPayloadSize) {
		return 0, nil, fmt.Errorf("varint value %d exceeds max payload", val)
	}
	return uint32(val), buf[n:], nil
}

// EncodeReqRespPayload encodes a request payload: varint(uncompressed_len) + snappy_framed(data).
// Uses snappy FRAMED format (not block) for req-resp cross-client compatibility.
func EncodeReqRespPayload(data []byte) []byte {
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

// DecodeReqRespPayload decodes a payload: varint(uncompressed_len) + snappy_framed(data).
// Uses snappy FRAMED format (not block) for req-resp cross-client compatibility.
func DecodeReqRespPayload(buf []byte) ([]byte, error) {
	declaredLen, rest, err := DecodeVarint(buf)
	if err != nil {
		return nil, fmt.Errorf("decode varint: %w", err)
	}

	// Try framed format first (cross-client), fall back to block format (self-to-self).
	r := snappy.NewReader(bytes.NewReader(rest))
	decoded, err := io.ReadAll(r)
	if err != nil {
		// Fallback: try block format (for self-to-self communication).
		decoded, err = snappy.Decode(nil, rest)
		if err != nil {
			return nil, fmt.Errorf("snappy decode: %w", err)
		}
	}

	if declaredLen > 0 && uint32(len(decoded)) != declaredLen {
		return nil, fmt.Errorf("length mismatch: declared %d, got %d", declaredLen, len(decoded))
	}
	return decoded, nil
}

// Response codes rs.
const (
	RespSuccess             byte = 0x00
	RespInvalidRequest      byte = 0x01
	RespServerError         byte = 0x02
	RespResourceUnavailable byte = 0x03
)

// EncodeResponse encodes a response chunk: code + varint(len) + snappy(data).
func EncodeResponse(code byte, data []byte) []byte {
	payload := EncodeReqRespPayload(data)
	result := make([]byte, 1+len(payload))
	result[0] = code
	copy(result[1:], payload)
	return result
}

// DecodeResponse reads a response chunk from a reader.
// Returns (code, decoded_payload, error).
func DecodeResponse(r io.Reader) (byte, []byte, error) {
	// Read response code.
	codeBuf := make([]byte, 1)
	if _, err := io.ReadFull(r, codeBuf); err != nil {
		return 0, nil, fmt.Errorf("read response code: %w", err)
	}
	code := codeBuf[0]

	// Read remaining bytes.
	rest, err := io.ReadAll(io.LimitReader(r, int64(MaxCompressedPayloadSize)))
	if err != nil {
		return code, nil, fmt.Errorf("read response payload: %w", err)
	}

	if len(rest) == 0 {
		return code, nil, nil
	}

	decoded, err := DecodeReqRespPayload(rest)
	if err != nil {
		return code, nil, fmt.Errorf("decode response payload: %w", err)
	}

	return code, decoded, nil
}
