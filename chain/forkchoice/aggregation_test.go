package forkchoice

import (
	"testing"

	"github.com/geanlabs/gean/chain/statetransition"
)

func TestBitlistToValidatorIDs(t *testing.T) {
	bits := statetransition.MakeBitlist(6)
	bits = statetransition.SetBit(bits, 0, true)
	bits = statetransition.SetBit(bits, 2, true)
	bits = statetransition.SetBit(bits, 5, true)

	got := bitlistToValidatorIDs(bits)
	want := []uint64{0, 2, 5}

	if len(got) != len(want) {
		t.Fatalf("unexpected length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected validator id at %d: got %d, want %d", i, got[i], want[i])
		}
	}
}

func TestBitlistsEqual(t *testing.T) {
	a := statetransition.MakeBitlist(4)
	a = statetransition.SetBit(a, 1, true)
	a = statetransition.SetBit(a, 3, true)

	b := statetransition.MakeBitlist(4)
	b = statetransition.SetBit(b, 1, true)
	b = statetransition.SetBit(b, 3, true)

	if !bitlistsEqual(a, b) {
		t.Fatal("expected bitlists to be equal by bit values")
	}

	b = statetransition.SetBit(b, 2, true)
	if bitlistsEqual(a, b) {
		t.Fatal("expected bitlists with different set bits to be unequal")
	}
}
