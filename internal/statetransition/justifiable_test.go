package statetransition

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"
)

func TestSlotIsJustifiableAfter(t *testing.T) {
	tests := []struct {
		slot, finalized uint64
		want            bool
	}{
		{1, 0, true},
		{5, 0, true},
		{6, 1, true},
		{9, 0, true},
		{16, 0, true},
		{25, 0, true},
		{36, 0, true},
		{100, 0, true},
		{6, 0, true},
		{12, 0, true},
		{20, 0, true},
		{30, 0, true},
		{42, 0, true},
		{7, 0, false},
		{8, 0, false},
		{10, 0, false},
		{11, 0, false},
		{13, 0, false},
		{14, 0, false},
		{15, 0, false},
		{17, 0, false},
		{18, 0, false},
		{19, 0, false},
		{21, 0, false},
		{0, 0, true},
		{10, 10, true},
		{5, 10, false},
	}

	for _, tt := range tests {
		got := SlotIsJustifiableAfter(tt.slot, tt.finalized)
		if got != tt.want {
			t.Errorf("SlotIsJustifiableAfter(%d, %d) = %v, want %v",
				tt.slot, tt.finalized, got, tt.want)
		}
	}
}

func TestIsSlotJustifiedIgnoresBitlistTerminator(t *testing.T) {
	state := makeGenesisState(1)
	state.LatestFinalized.Slot = 0

	// An empty justified bitlist tracks no slots: slot 1 is out of range rather
	// than spuriously justified by the SSZ terminator bit.
	if justified, err := IsSlotJustified(state, 0, 1); justified || err == nil {
		t.Fatalf("empty justified bitlist: got justified=%v err=%v, want false + out-of-range error", justified, err)
	}

	setSlotJustified(state, 0, 1)
	if justified, err := IsSlotJustified(state, 0, 1); !justified || err != nil {
		t.Fatalf("explicitly justified slot 1: got justified=%v err=%v, want true + nil", justified, err)
	}
}

func TestIsqrtMatchesBigInt(t *testing.T) {
	check := func(n uint64) {
		t.Helper()
		want := new(big.Int).Sqrt(new(big.Int).SetUint64(n)).Uint64()
		if got := isqrt(n); got != want {
			t.Fatalf("isqrt(%d) = %d, want %d", n, got, want)
		}
	}
	for n := uint64(0); n < 1<<16; n++ {
		check(n)
	}
	edges := []uint64{
		math.MaxUint64, math.MaxUint64 - 1,
		(1<<32 - 1) * (1<<32 - 1), (1<<32-1)*(1<<32-1) - 1, (1<<32-1)*(1<<32-1) + 1,
		1 << 53, 1<<53 + 1, 1<<62 - 1, 1 << 62,
	}
	for _, n := range edges {
		check(n)
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for range 100000 {
		check(rng.Uint64())
	}
}

// justifiableReference mirrors leanSpec slot.py with unbounded integers.
func justifiableReference(slot, finalized uint64) bool {
	if slot < finalized {
		return false
	}
	delta := new(big.Int).SetUint64(slot - finalized)
	if delta.Cmp(big.NewInt(5)) <= 0 {
		return true
	}
	r := new(big.Int).Sqrt(delta)
	if new(big.Int).Mul(r, r).Cmp(delta) == 0 {
		return true
	}
	disc := new(big.Int).Add(new(big.Int).Mul(delta, big.NewInt(4)), big.NewInt(1))
	dr := new(big.Int).Sqrt(disc)
	return new(big.Int).Mul(dr, dr).Cmp(disc) == 0 && dr.Bit(0) == 1
}

func TestSlotIsJustifiableAfterMatchesSpecReference(t *testing.T) {
	for delta := uint64(0); delta < 1<<20; delta++ {
		if got, want := SlotIsJustifiableAfter(delta+7, 7), justifiableReference(delta+7, 7); got != want {
			t.Fatalf("delta %d: got %v, want %v", delta, got, want)
		}
	}
	// Large distances where float64 rounding and 4*delta+1 overflow would bite.
	n := uint64(1<<32 - 1)
	for _, delta := range []uint64{
		n * n, n*n - 1, n * (n - 1), n*(n-1) + 1, (n - 1) * (n - 1),
		1<<53 + 1, (1<<26 + 1) * (1<<26 + 1), (1<<27 + 3) * (1<<27 + 4),
		math.MaxUint64, 1<<62 + 1,
	} {
		if got, want := SlotIsJustifiableAfter(delta, 0), justifiableReference(delta, 0); got != want {
			t.Fatalf("delta %d: got %v, want %v", delta, got, want)
		}
	}
}
