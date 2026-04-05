package types

// Bitlist helpers for working with SSZ bitlists encoded as []byte.
// SSZ bitlists use a delimiter bit: the highest set bit marks the end of data.

// BitlistLen returns the number of data bits in a SSZ-encoded bitlist.
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

// BitlistGet returns true if bit at index i is set.
func BitlistGet(b []byte, i uint64) bool {
	if i/8 >= uint64(len(b)) {
		return false
	}
	return b[i/8]&(1<<(i%8)) != 0
}

// BitlistSet sets bit at index i to 1.
func BitlistSet(b []byte, i uint64) {
	if i/8 >= uint64(len(b)) {
		return
	}
	b[i/8] |= 1 << (i % 8)
}

// BitlistCount returns the number of set data bits.
func BitlistCount(b []byte) uint64 {
	length := BitlistLen(b)
	var count uint64
	for i := uint64(0); i < length; i++ {
		if BitlistGet(b, i) {
			count++
		}
	}
	return count
}

// NewBitlistSSZ creates a new SSZ-encoded bitlist with the given number of data bits.
func NewBitlistSSZ(length uint64) []byte {
	if length == 0 {
		return []byte{0x01}
	}
	numBytes := (length + 8) / 8
	data := make([]byte, numBytes)
	data[length/8] |= 1 << (length % 8)
	return data
}

// BitlistIndices returns all set bit indices in a bitlist.
func BitlistIndices(b []byte) []uint64 {
	length := BitlistLen(b)
	var indices []uint64
	for i := uint64(0); i < length; i++ {
		if BitlistGet(b, i) {
			indices = append(indices, i)
		}
	}
	return indices
}

// BitlistExtend grows a bitlist to newLen, preserving existing bits.
func BitlistExtend(b []byte, newLen uint64) []byte {
	oldLen := BitlistLen(b)
	if newLen <= oldLen {
		return b
	}
	// Clear old delimiter
	if oldLen < uint64(len(b))*8 {
		b[oldLen/8] &^= 1 << (oldLen % 8)
	}
	// Grow
	needed := (newLen + 8) / 8
	for uint64(len(b)) < needed {
		b = append(b, 0)
	}
	// New delimiter
	b[newLen/8] |= 1 << (newLen % 8)
	return b
}
