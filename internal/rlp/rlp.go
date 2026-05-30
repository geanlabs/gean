package rlp

import (
	"encoding/binary"
	"fmt"
)

type valueKind byte

const (
	stringKind valueKind = iota
	listKind
)

// SplitList decodes a top-level RLP list and returns each element as its raw
// encoded RLP value.
func SplitList(input []byte) ([][]byte, error) {
	kind, payload, rest, err := split(input)
	if err != nil {
		return nil, err
	}
	if kind != listKind {
		return nil, fmt.Errorf("rlp: expected list")
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("rlp: trailing data")
	}

	items := make([][]byte, 0)
	for len(payload) > 0 {
		_, _, next, err := split(payload)
		if err != nil {
			return nil, err
		}
		rawLen := len(payload) - len(next)
		items = append(items, payload[:rawLen])
		payload = next
	}
	return items, nil
}

// Bytes decodes one RLP string value.
func Bytes(input []byte) ([]byte, error) {
	kind, payload, rest, err := split(input)
	if err != nil {
		return nil, err
	}
	if kind != stringKind {
		return nil, fmt.Errorf("rlp: expected string")
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("rlp: trailing data")
	}
	return payload, nil
}

// String decodes one RLP string value as a Go string.
func String(input []byte) (string, error) {
	b, err := Bytes(input)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Uint64 decodes one RLP unsigned integer.
func Uint64(input []byte) (uint64, error) {
	b, err := Bytes(input)
	if err != nil {
		return 0, err
	}
	if len(b) == 0 {
		return 0, nil
	}
	if len(b) > 8 {
		return 0, fmt.Errorf("rlp: uint64 overflow")
	}
	if len(b) > 1 && b[0] == 0 {
		return 0, fmt.Errorf("rlp: non-minimal integer")
	}
	var buf [8]byte
	copy(buf[8-len(b):], b)
	return binary.BigEndian.Uint64(buf[:]), nil
}

// Uint16 decodes one RLP unsigned integer and verifies it fits in 16 bits.
func Uint16(input []byte) (uint16, error) {
	v, err := Uint64(input)
	if err != nil {
		return 0, err
	}
	if v > uint64(^uint16(0)) {
		return 0, fmt.Errorf("rlp: uint16 overflow")
	}
	return uint16(v), nil
}

// EncodeBytes encodes a byte string as RLP.
func EncodeBytes(payload []byte) []byte {
	if len(payload) == 1 && payload[0] < 0x80 {
		return []byte{payload[0]}
	}
	return encodePayload(0x80, 0xb7, payload)
}

// EncodeUint64 encodes an unsigned integer as RLP.
func EncodeUint64(v uint64) []byte {
	if v == 0 {
		return []byte{0x80}
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	i := 0
	for i < len(buf) && buf[i] == 0 {
		i++
	}
	return EncodeBytes(buf[i:])
}

// EncodeUint16 encodes a uint16 as RLP.
func EncodeUint16(v uint16) []byte {
	return EncodeUint64(uint64(v))
}

// EncodeList encodes already-encoded RLP values as an RLP list.
func EncodeList(items ...[]byte) []byte {
	payloadLen := 0
	for _, item := range items {
		payloadLen += len(item)
	}
	payload := make([]byte, 0, payloadLen)
	for _, item := range items {
		payload = append(payload, item...)
	}
	return encodePayload(0xc0, 0xf7, payload)
}

func split(input []byte) (valueKind, []byte, []byte, error) {
	if len(input) == 0 {
		return 0, nil, nil, fmt.Errorf("rlp: empty input")
	}

	prefix := input[0]
	switch {
	case prefix < 0x80:
		return stringKind, input[:1], input[1:], nil
	case prefix <= 0xb7:
		size := int(prefix - 0x80)
		if len(input) < 1+size {
			return 0, nil, nil, fmt.Errorf("rlp: short string")
		}
		if size == 1 && input[1] < 0x80 {
			return 0, nil, nil, fmt.Errorf("rlp: non-minimal string")
		}
		return stringKind, input[1 : 1+size], input[1+size:], nil
	case prefix <= 0xbf:
		sizeOfSize := int(prefix - 0xb7)
		size, err := readSize(input[1:], sizeOfSize)
		if err != nil {
			return 0, nil, nil, err
		}
		if size <= 55 {
			return 0, nil, nil, fmt.Errorf("rlp: non-minimal long string")
		}
		offset := 1 + sizeOfSize
		if len(input) < offset+size {
			return 0, nil, nil, fmt.Errorf("rlp: short long string")
		}
		return stringKind, input[offset : offset+size], input[offset+size:], nil
	case prefix <= 0xf7:
		size := int(prefix - 0xc0)
		if len(input) < 1+size {
			return 0, nil, nil, fmt.Errorf("rlp: short list")
		}
		return listKind, input[1 : 1+size], input[1+size:], nil
	default:
		sizeOfSize := int(prefix - 0xf7)
		size, err := readSize(input[1:], sizeOfSize)
		if err != nil {
			return 0, nil, nil, err
		}
		if size <= 55 {
			return 0, nil, nil, fmt.Errorf("rlp: non-minimal long list")
		}
		offset := 1 + sizeOfSize
		if len(input) < offset+size {
			return 0, nil, nil, fmt.Errorf("rlp: short long list")
		}
		return listKind, input[offset : offset+size], input[offset+size:], nil
	}
}

func readSize(input []byte, sizeOfSize int) (int, error) {
	if sizeOfSize == 0 || sizeOfSize > 8 {
		return 0, fmt.Errorf("rlp: invalid length size")
	}
	if len(input) < sizeOfSize {
		return 0, fmt.Errorf("rlp: short length")
	}
	if input[0] == 0 {
		return 0, fmt.Errorf("rlp: non-minimal length")
	}
	var size uint64
	for _, b := range input[:sizeOfSize] {
		size = size<<8 | uint64(b)
	}
	if size > uint64(int(^uint(0)>>1)) {
		return 0, fmt.Errorf("rlp: length overflow")
	}
	return int(size), nil
}

func encodePayload(shortBase, longBase byte, payload []byte) []byte {
	if len(payload) <= 55 {
		out := make([]byte, 1+len(payload))
		out[0] = shortBase + byte(len(payload))
		copy(out[1:], payload)
		return out
	}

	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(payload)))
	i := 0
	for i < len(lenBuf) && lenBuf[i] == 0 {
		i++
	}
	sizeBytes := lenBuf[i:]
	out := make([]byte, 1+len(sizeBytes)+len(payload))
	out[0] = longBase + byte(len(sizeBytes))
	copy(out[1:], sizeBytes)
	copy(out[1+len(sizeBytes):], payload)
	return out
}
