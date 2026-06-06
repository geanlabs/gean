package blockprocessor

import "testing"

func TestSlot32RejectsOverflow(t *testing.T) {
	if _, err := slot32(^uint64(0)); err == nil {
		t.Fatal("expected overflow error")
	}
}

func TestSlot32AcceptsUint32Max(t *testing.T) {
	got, err := slot32(uint64(^uint32(0)))
	if err != nil {
		t.Fatalf("slot32: %v", err)
	}
	if got != ^uint32(0) {
		t.Fatalf("slot=%d, want %d", got, ^uint32(0))
	}
}
