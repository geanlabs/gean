package types

import "testing"

func TestNewBitlistSSZ(t *testing.T) {
	bl := NewBitlistSSZ(8)
	if BitlistLen(bl) != 8 {
		t.Fatalf("expected length 8, got %d", BitlistLen(bl))
	}
	for i := uint64(0); i < 8; i++ {
		if BitlistGet(bl, i) {
			t.Fatalf("bit %d should be false", i)
		}
	}
}

func TestBitlistSetAndGet(t *testing.T) {
	bl := NewBitlistSSZ(16)
	BitlistSet(bl, 0)
	BitlistSet(bl, 5)
	BitlistSet(bl, 15)
	if !BitlistGet(bl, 0) || !BitlistGet(bl, 5) || !BitlistGet(bl, 15) {
		t.Fatal("set bits not readable")
	}
	if BitlistGet(bl, 1) || BitlistGet(bl, 14) {
		t.Fatal("unset bits should be false")
	}
	if BitlistCount(bl) != 3 {
		t.Fatalf("expected count 3, got %d", BitlistCount(bl))
	}
}

func TestBitlistExtend(t *testing.T) {
	bl := NewBitlistSSZ(4)
	BitlistSet(bl, 1)
	BitlistSet(bl, 3)
	bl = BitlistExtend(bl, 10)
	if BitlistLen(bl) != 10 {
		t.Fatalf("expected length 10, got %d", BitlistLen(bl))
	}
	if !BitlistGet(bl, 1) || !BitlistGet(bl, 3) {
		t.Fatal("original bits lost after extend")
	}
	if BitlistGet(bl, 5) {
		t.Fatal("extended bits should be false")
	}
}

func TestBitlistEmpty(t *testing.T) {
	bl := NewBitlistSSZ(0)
	if BitlistLen(bl) != 0 {
		t.Fatalf("expected length 0, got %d", BitlistLen(bl))
	}
	if BitlistCount(bl) != 0 {
		t.Fatalf("expected count 0, got %d", BitlistCount(bl))
	}
}

func TestBitlistFromRawBytes(t *testing.T) {
	// 3 bits [true, false, true] + delimiter = 0b00001101 = 0x0d
	bl := []byte{0x0d}
	if BitlistLen(bl) != 3 {
		t.Fatalf("expected length 3, got %d", BitlistLen(bl))
	}
	if !BitlistGet(bl, 0) || BitlistGet(bl, 1) || !BitlistGet(bl, 2) {
		t.Fatal("incorrect bits")
	}
}
