package unit

import (
	"testing"

	"github.com/geanlabs/gean/chain/statetransition"
)

func TestBitlistLenEmpty(t *testing.T) {
	if got := statetransition.BitlistLen(nil); got != 0 {
		t.Fatalf("BitlistLen(nil) = %d, want 0", got)
	}
	if got := statetransition.BitlistLen([]byte{}); got != 0 {
		t.Fatalf("BitlistLen([]) = %d, want 0", got)
	}
}

func TestBitlistLenSentinelOnly(t *testing.T) {
	if got := statetransition.BitlistLen([]byte{0x01}); got != 0 {
		t.Fatalf("BitlistLen([0x01]) = %d, want 0", got)
	}
}

func TestBitlistLenOneBit(t *testing.T) {
	if got := statetransition.BitlistLen([]byte{0x02}); got != 1 {
		t.Fatalf("BitlistLen([0x02]) = %d, want 1", got)
	}
	if got := statetransition.BitlistLen([]byte{0x03}); got != 1 {
		t.Fatalf("BitlistLen([0x03]) = %d, want 1", got)
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
		if got := statetransition.BitlistLen(tt.bl); got != tt.want {
			t.Errorf("%s: BitlistLen = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestGetBit(t *testing.T) {
	bl := []byte{0x05}
	if !statetransition.GetBit(bl, 0) {
		t.Error("bit 0 should be true")
	}
	if statetransition.GetBit(bl, 1) {
		t.Error("bit 1 should be false")
	}
}

func TestGetBitOutOfBounds(t *testing.T) {
	bl := []byte{0x03}
	if statetransition.GetBit(bl, 100) {
		t.Error("out-of-bounds bit should return false")
	}
}

func TestSetBit(t *testing.T) {
	bl := []byte{0x04}

	bl = statetransition.SetBit(bl, 0, true)
	if !statetransition.GetBit(bl, 0) {
		t.Error("bit 0 should be set after SetBit(0, true)")
	}

	bl = statetransition.SetBit(bl, 0, false)
	if statetransition.GetBit(bl, 0) {
		t.Error("bit 0 should be clear after SetBit(0, false)")
	}
}

func TestSetBitOutOfBounds(t *testing.T) {
	bl := []byte{0x03}
	result := statetransition.SetBit(bl, 100, true)
	if len(result) != 1 {
		t.Error("out-of-bounds SetBit should not modify slice length")
	}
}

func TestAppendBitFromEmpty(t *testing.T) {
	bl := []byte{0x01}

	bl = statetransition.AppendBit(bl, true)
	if statetransition.BitlistLen(bl) != 1 {
		t.Fatalf("after 1 append: len = %d, want 1", statetransition.BitlistLen(bl))
	}
	if !statetransition.GetBit(bl, 0) {
		t.Error("bit 0 should be true")
	}

	bl = statetransition.AppendBit(bl, false)
	if statetransition.BitlistLen(bl) != 2 {
		t.Fatalf("after 2 appends: len = %d, want 2", statetransition.BitlistLen(bl))
	}
	if statetransition.GetBit(bl, 1) {
		t.Error("bit 1 should be false")
	}
}

func TestAppendBitCrossesByteBoundary(t *testing.T) {
	bl := []byte{0x01}
	for i := 0; i < 8; i++ {
		bl = statetransition.AppendBit(bl, i%2 == 0)
	}
	if statetransition.BitlistLen(bl) != 8 {
		t.Fatalf("len = %d, want 8", statetransition.BitlistLen(bl))
	}
	for i := 0; i < 8; i++ {
		expected := i%2 == 0
		if statetransition.GetBit(bl, uint64(i)) != expected {
			t.Errorf("bit %d = %v, want %v", i, statetransition.GetBit(bl, uint64(i)), expected)
		}
	}

	bl = statetransition.AppendBit(bl, true)
	if statetransition.BitlistLen(bl) != 9 {
		t.Fatalf("len = %d, want 9", statetransition.BitlistLen(bl))
	}
	if !statetransition.GetBit(bl, 8) {
		t.Error("bit 8 should be true")
	}
}

func TestAppendBitMany(t *testing.T) {
	bl := []byte{0x01}
	n := 64
	for i := 0; i < n; i++ {
		bl = statetransition.AppendBit(bl, true)
	}
	if statetransition.BitlistLen(bl) != n {
		t.Fatalf("len = %d, want %d", statetransition.BitlistLen(bl), n)
	}
	for i := 0; i < n; i++ {
		if !statetransition.GetBit(bl, uint64(i)) {
			t.Fatalf("bit %d should be true", i)
		}
	}
}

func TestBitlistRoundTrip(t *testing.T) {
	bl := []byte{0x01}
	values := []bool{true, false, true, true, false, false, true, false, true}
	for _, v := range values {
		bl = statetransition.AppendBit(bl, v)
	}
	if statetransition.BitlistLen(bl) != len(values) {
		t.Fatalf("len = %d, want %d", statetransition.BitlistLen(bl), len(values))
	}
	for i, expected := range values {
		if statetransition.GetBit(bl, uint64(i)) != expected {
			t.Errorf("bit %d = %v, want %v", i, statetransition.GetBit(bl, uint64(i)), expected)
		}
	}
}

func TestBitlistLenZeroLastByte(t *testing.T) {
	if got := statetransition.BitlistLen([]byte{0xff, 0x00}); got != 0 {
		t.Fatalf("BitlistLen with zero last byte = %d, want 0", got)
	}
}
