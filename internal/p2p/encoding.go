package p2p

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

func SnappyRawEncode(data []byte) []byte {
	return snappy.Encode(nil, data)
}

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

func EncodeVarint(value uint32) []byte {
	buf := make([]byte, binary.MaxVarintLen32)
	n := binary.PutUvarint(buf, uint64(value))
	return buf[:n]
}

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

func DecodeReqRespPayload(buf []byte) ([]byte, error) {
	declaredLen, rest, err := DecodeVarint(buf)
	if err != nil {
		return nil, fmt.Errorf("decode varint: %w", err)
	}

	r := snappy.NewReader(bytes.NewReader(rest))
	decoded, err := io.ReadAll(io.LimitReader(r, int64(declaredLen)+1))
	if err != nil {
		return nil, fmt.Errorf("snappy framed decode: %w", err)
	}

	if len(decoded) > MaxPayloadSize {
		return nil, fmt.Errorf("decoded payload %d exceeds max %d", len(decoded), MaxPayloadSize)
	}
	if uint32(len(decoded)) != declaredLen {
		return nil, fmt.Errorf("length mismatch: declared %d, got %d", declaredLen, len(decoded))
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
	payload := EncodeReqRespPayload(data)
	result := make([]byte, 1+len(payload))
	result[0] = code
	copy(result[1:], payload)
	return result
}

func DecodeResponse(r io.Reader) (byte, []byte, error) {
	br, ok := r.(interface {
		io.Reader
		io.ByteReader
	})
	if !ok {
		br = bufio.NewReader(r)
	}

	code, err := br.ReadByte()
	if err != nil {
		return 0, nil, fmt.Errorf("read response code: %w", err)
	}

	declaredLen, err := decodeVarintFrom(br)
	if err != nil {
		return code, nil, fmt.Errorf("decode response length: %w", err)
	}
	if declaredLen > MaxPayloadSize {
		return code, nil, fmt.Errorf("response length %d exceeds max %d", declaredLen, MaxPayloadSize)
	}

	decoded := make([]byte, declaredLen)
	sr := snappy.NewReader(br)
	if declaredLen > 0 {
		if _, err := io.ReadFull(sr, decoded); err != nil {
			return code, nil, fmt.Errorf("decode response payload: %w", err)
		}
	} else {
		var scratch [1]byte
		if n, err := sr.Read(scratch[:]); err != io.EOF {
			if err != nil {
				return code, nil, fmt.Errorf("decode empty response payload: %w", err)
			}
			return code, nil, fmt.Errorf("length mismatch: declared 0, got at least %d", n)
		}
	}

	if code != RespSuccess && len(decoded) > MaxErrorMessageSize {
		return code, nil, fmt.Errorf("error message %d bytes exceeds MaxErrorMessageSize %d", len(decoded), MaxErrorMessageSize)
	}

	return code, decoded, nil
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
