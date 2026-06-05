package types

import "testing"

func TestNewBitlistSSZ(t *testing.T) {
	bl := NewBitlistSSZ(8)
	if BitlistLen(bl) != 8 {
		t.Fatalf("expected length 8, got %d", BitlistLen(bl))
	}
	for i := range uint64(8) {
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

func TestBitlistSetDoesNotMoveDelimiter(t *testing.T) {
	bl := NewBitlistSSZ(4)
	BitlistSet(bl, 4)
	BitlistSet(bl, 7)
	if got := BitlistLen(bl); got != 4 {
		t.Fatalf("BitlistLen after out-of-range set=%d, want 4", got)
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
	if BitlistGet(bl, 0) {
		t.Fatal("terminator bit must not be exposed as data")
	}
}

func TestBitlistFromRawBytes(t *testing.T) {
	bl := []byte{0x0d}
	if BitlistLen(bl) != 3 {
		t.Fatalf("expected length 3, got %d", BitlistLen(bl))
	}
	if !BitlistGet(bl, 0) || BitlistGet(bl, 1) || !BitlistGet(bl, 2) {
		t.Fatal("incorrect bits")
	}
}

func TestBitlistIndices(t *testing.T) {
	bl := NewBitlistSSZ(8)
	BitlistSet(bl, 0)
	BitlistSet(bl, 3)
	BitlistSet(bl, 7)

	got := BitlistIndices(bl)
	want := []uint64{0, 3, 7}
	if len(got) != len(want) {
		t.Fatalf("indices=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("indices=%v, want %v", got, want)
		}
	}
}

func TestBitlistFromIndices(t *testing.T) {
	bits := BitlistFromIndices([]uint64{0, 3, 7})
	if !BitlistGet(bits, 0) || !BitlistGet(bits, 3) || !BitlistGet(bits, 7) {
		t.Fatal("expected bits 0, 3, 7 set")
	}
	if BitlistGet(bits, 1) || BitlistGet(bits, 5) {
		t.Fatal("bits 1, 5 should not be set")
	}
	if BitlistLen(bits) != 8 {
		t.Fatalf("expected length 8, got %d", BitlistLen(bits))
	}
}

func TestBitlistHandlesUnrepresentableLengths(t *testing.T) {
	bits := NewBitlistSSZ(^uint64(0))
	if BitlistLen(bits) != 0 {
		t.Fatalf("unrepresentable bitlist length=%d, want 0", BitlistLen(bits))
	}

	bits = NewBitlistSSZ(1)
	BitlistSet(bits, 0)
	extended := BitlistExtend(bits, ^uint64(0))
	if BitlistLen(extended) != 1 || !BitlistGet(extended, 0) {
		t.Fatalf("overflow extend changed bitlist: len=%d bits=%x", BitlistLen(extended), extended)
	}

	bits = BitlistFromIndices([]uint64{^uint64(0)})
	if BitlistLen(bits) != 0 || BitlistCount(bits) != 0 {
		t.Fatalf("unrepresentable indices produced len=%d count=%d", BitlistLen(bits), BitlistCount(bits))
	}
}
