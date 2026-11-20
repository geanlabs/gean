package types

const maxBitlistBytes = JustificationValidatorsLimit/8 + 1

func BitlistLen(b []byte) uint64 {
	if len(b) == 0 {
		return 0
	}
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == 0 {
			continue
		}
		for bit := 7; bit >= 0; bit-- {
			if b[i]&(1<<uint(bit)) != 0 {
				return uint64(i)*8 + uint64(bit)
			}
		}
	}
	return 0
}

func BitlistGet(b []byte, i uint64) bool {
	if i >= BitlistLen(b) {
		return false
	}
	return bitlistRawGet(b, i)
}

func bitlistRawGet(b []byte, i uint64) bool {
	if i/8 >= uint64(len(b)) {
		return false
	}
	return b[i/8]&(1<<(i%8)) != 0
}

func BitlistSet(b []byte, i uint64) {
	if i >= BitlistLen(b) || i/8 >= uint64(len(b)) {
		return
	}
	b[i/8] |= 1 << (i % 8)
}

func BitlistCount(b []byte) uint64 {
	length := BitlistLen(b)
	var count uint64
	for i := range length {
		if bitlistRawGet(b, i) {
			count++
		}
	}
	return count
}

func NewBitlistSSZ(length uint64) []byte {
	numBytes, ok := bitlistByteLen(length)
	if !ok {
		return []byte{0x01}
	}
	data := make([]byte, numBytes)
	data[length/8] |= 1 << (length % 8)
	return data
}

func BitlistFromIndices(ids []uint64) []byte {
	if len(ids) == 0 {
		return NewBitlistSSZ(0)
	}
	maxID := uint64(0)
	for _, id := range ids {
		if id > maxID {
			maxID = id
		}
	}
	if maxID == ^uint64(0) {
		return NewBitlistSSZ(0)
	}
	bits := NewBitlistSSZ(maxID + 1)
	for _, id := range ids {
		BitlistSet(bits, id)
	}
	return bits
}

func BitlistIndices(b []byte) []uint64 {
	length := BitlistLen(b)
	var indices []uint64
	for i := range length {
		if bitlistRawGet(b, i) {
			indices = append(indices, i)
		}
	}
	return indices
}

func BitlistExtend(b []byte, newLen uint64) []byte {
	oldLen := BitlistLen(b)
	if newLen <= oldLen {
		return b
	}
	needed, ok := bitlistByteLen(newLen)
	if !ok {
		return b
	}
	if oldLen < uint64(len(b))*8 {
		b[oldLen/8] &^= 1 << (oldLen % 8)
	}
	for len(b) < needed {
		b = append(b, 0)
	}
	b[newLen/8] |= 1 << (newLen % 8)
	return b
}

func bitlistByteLen(length uint64) (int, bool) {
	n := length/8 + 1
	if n > maxBitlistBytes {
		return 0, false
	}
	return int(n), true
}
