package xmss

import (
	"math"
	"testing"
)

func TestSlot(t *testing.T) {
	if got, err := Slot(math.MaxUint32); err != nil || got != math.MaxUint32 {
		t.Fatalf("Slot(MaxUint32) = %d, %v", got, err)
	}
	if _, err := Slot(math.MaxUint32 + 1); err == nil {
		t.Fatal("Slot(MaxUint32+1) did not overflow")
	}
}
