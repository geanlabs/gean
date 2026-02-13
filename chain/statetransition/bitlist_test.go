package statetransition

import "testing"

func TestBitlistLenEmpty(t *testing.T) {
	if got := bitlistLen(nil); got != 0 {
		t.Fatalf("bitlistLen(nil) = %d, want 0", got)
	}
	if got := bitlistLen([]byte{}); got != 0 {
		t.Fatalf("bitlistLen([]) = %d, want 0", got)
	}
}

func TestBitlistLenSentinelOnly(t *testing.T) {
	if got := bitlistLen([]byte{0x01}); got != 0 {
		t.Fatalf("bitlistLen([0x01]) = %d, want 0", got)
	}
}

func TestBitlistLenOneBit(t *testing.T) {
	if got := bitlistLen([]byte{0x02}); got != 1 {
		t.Fatalf("bitlistLen([0x02]) = %d, want 1", got)
	}
	if got := bitlistLen([]byte{0x03}); got != 1 {
		t.Fatalf("bitlistLen([0x03]) = %d, want 1", got)
	}
}

func TestBitlistLenMultipleBits(t *testing.T) {
	tests := []struct {
		name string
		bl   []byte
		want int
	}{
		{"2 bits", []byte{0x04}, 2},
		{"3 bits", []byte{0x08}, 3},
		{"7 bits", []byte{0x80}, 7},
		{"8 bits", []byte{0x00, 0x01}, 8},
		{"9 bits", []byte{0x00, 0x02}, 9},
		{"16 bits", []byte{0x00, 0x00, 0x01}, 16},
	}
	for _, tt := range tests {
		if got := bitlistLen(tt.bl); got != tt.want {
			t.Errorf("%s: bitlistLen = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestGetBit(t *testing.T) {
	bl := []byte{0x05}
	if !getBit(bl, 0) {
		t.Error("bit 0 should be true")
	}
	if getBit(bl, 1) {
		t.Error("bit 1 should be false")
	}
}

func TestGetBitOutOfBounds(t *testing.T) {
	bl := []byte{0x03}
	if getBit(bl, 100) {
		t.Error("out-of-bounds bit should return false")
	}
}

func TestSetBit(t *testing.T) {
	bl := []byte{0x04}

	bl = setBit(bl, 0, true)
	if !getBit(bl, 0) {
		t.Error("bit 0 should be set after setBit(0, true)")
	}

	bl = setBit(bl, 0, false)
	if getBit(bl, 0) {
		t.Error("bit 0 should be clear after setBit(0, false)")
	}
}

func TestSetBitOutOfBounds(t *testing.T) {
	bl := []byte{0x03}
	result := setBit(bl, 100, true)
	if len(result) != 1 {
		t.Error("out-of-bounds setBit should not modify slice length")
	}
}

func TestAppendBitFromEmpty(t *testing.T) {
	bl := []byte{0x01}

	bl = appendBit(bl, true)
	if bitlistLen(bl) != 1 {
		t.Fatalf("after 1 append: len = %d, want 1", bitlistLen(bl))
	}
	if !getBit(bl, 0) {
		t.Error("bit 0 should be true")
	}

	bl = appendBit(bl, false)
	if bitlistLen(bl) != 2 {
		t.Fatalf("after 2 appends: len = %d, want 2", bitlistLen(bl))
	}
	if getBit(bl, 1) {
		t.Error("bit 1 should be false")
	}
}

func TestAppendBitCrossesByteBoundary(t *testing.T) {
	bl := []byte{0x01}
	for i := 0; i < 8; i++ {
		bl = appendBit(bl, i%2 == 0)
	}
	if bitlistLen(bl) != 8 {
		t.Fatalf("len = %d, want 8", bitlistLen(bl))
	}
	for i := 0; i < 8; i++ {
		expected := i%2 == 0
		if getBit(bl, uint64(i)) != expected {
			t.Errorf("bit %d = %v, want %v", i, getBit(bl, uint64(i)), expected)
		}
	}

	bl = appendBit(bl, true)
	if bitlistLen(bl) != 9 {
		t.Fatalf("len = %d, want 9", bitlistLen(bl))
	}
	if !getBit(bl, 8) {
		t.Error("bit 8 should be true")
	}
}

func TestAppendBitMany(t *testing.T) {
	bl := []byte{0x01}
	n := 64
	for i := 0; i < n; i++ {
		bl = appendBit(bl, true)
	}
	if bitlistLen(bl) != n {
		t.Fatalf("len = %d, want %d", bitlistLen(bl), n)
	}
	for i := 0; i < n; i++ {
		if !getBit(bl, uint64(i)) {
			t.Fatalf("bit %d should be true", i)
		}
	}
}

func TestBitlistRoundTrip(t *testing.T) {
	bl := []byte{0x01}
	values := []bool{true, false, true, true, false, false, true, false, true}
	for _, v := range values {
		bl = appendBit(bl, v)
	}
	if bitlistLen(bl) != len(values) {
		t.Fatalf("len = %d, want %d", bitlistLen(bl), len(values))
	}
	for i, expected := range values {
		if getBit(bl, uint64(i)) != expected {
			t.Errorf("bit %d = %v, want %v", i, getBit(bl, uint64(i)), expected)
		}
	}
}

func TestBitlistLenZeroLastByte(t *testing.T) {
	if got := bitlistLen([]byte{0xff, 0x00}); got != 0 {
		t.Fatalf("bitlistLen with zero last byte = %d, want 0", got)
	}
}
