package statetransition

// SSZ bitlist helpers.
//
// Bits are packed LSB-first into bytes. A sentinel '1' bit is appended
// after the last data bit to mark the length. The byte length is
// ceil((numBits + 1) / 8).

// bitlistLen returns the number of data bits in an SSZ bitlist.
func bitlistLen(bl []byte) int {
	if len(bl) == 0 {
		return 0
	}
	lastByte := bl[len(bl)-1]
	if lastByte == 0 {
		return 0
	}
	msb := 0
	for b := lastByte; b > 0; b >>= 1 {
		msb++
	}
	return (len(bl)-1)*8 + msb - 1
}

// getBit returns the value of bit at index idx in an SSZ bitlist.
func getBit(bl []byte, idx uint64) bool {
	byteIdx := idx / 8
	bitIdx := idx % 8
	if int(byteIdx) >= len(bl) {
		return false
	}
	return (bl[byteIdx] & (1 << bitIdx)) != 0
}

// setBit sets the value of bit at index idx in an SSZ bitlist.
func setBit(bl []byte, idx uint64, val bool) []byte {
	byteIdx := idx / 8
	bitIdx := idx % 8
	if int(byteIdx) >= len(bl) {
		return bl
	}
	if val {
		bl[byteIdx] |= 1 << bitIdx
	} else {
		bl[byteIdx] &^= 1 << bitIdx
	}
	return bl
}

// appendBit adds a new data bit to an SSZ bitlist, maintaining the sentinel.
func appendBit(bl []byte, val bool) []byte {
	n := bitlistLen(bl)
	newLen := n + 1
	neededBytes := (newLen + 1 + 7) / 8

	for len(bl) < neededBytes {
		bl = append(bl, 0)
	}
	bl = bl[:neededBytes]

	// Clear old sentinel.
	if n > 0 {
		sentinelByte := n / 8
		sentinelBit := n % 8
		if sentinelByte < len(bl) {
			bl[sentinelByte] &^= 1 << uint(sentinelBit)
		}
	}

	// Set the new data bit.
	dataByte := n / 8
	dataBit := n % 8
	if val {
		bl[dataByte] |= 1 << uint(dataBit)
	} else {
		bl[dataByte] &^= 1 << uint(dataBit)
	}

	// Set new sentinel at position newLen.
	sentinelByte := newLen / 8
	sentinelBit := newLen % 8
	bl[sentinelByte] |= 1 << uint(sentinelBit)

	return bl
}

// cloneBitlist returns a copy of an SSZ bitlist.
func cloneBitlist(src []byte) []byte {
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}
