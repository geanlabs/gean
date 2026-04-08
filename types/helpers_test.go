package types

import "testing"

func TestIsProposer(t *testing.T) {
	tests := []struct {
		slot, validator, numValidators uint64
		want                           bool
	}{
		{0, 0, 5, true},
		{1, 1, 5, true},
		{4, 4, 5, true},
		{5, 0, 5, true}, // wraps around
		{6, 1, 5, true},
		{0, 1, 5, false},
		{1, 0, 5, false},
		{0, 0, 0, false}, // no validators
	}
	for _, tt := range tests {
		got := IsProposer(tt.slot, tt.validator, tt.numValidators)
		if got != tt.want {
			t.Errorf("IsProposer(%d, %d, %d) = %v, want %v",
				tt.slot, tt.validator, tt.numValidators, got, tt.want)
		}
	}
}

func TestProposerIndex(t *testing.T) {
	if ProposerIndex(7, 3) != 1 {
		t.Fatal("expected proposer 1 for slot 7 with 3 validators")
	}
	if ProposerIndex(0, 0) != -1 {
		t.Fatal("expected -1 for 0 validators")
	}
}

func TestIsZeroRoot(t *testing.T) {
	if !IsZeroRoot(ZeroRoot) {
		t.Fatal("ZeroRoot should be zero")
	}
	nonZero := [RootSize]byte{1}
	if IsZeroRoot(nonZero) {
		t.Fatal("non-zero root should not be zero")
	}
}

func TestShortRoot(t *testing.T) {
	root := [RootSize]byte{0xab, 0xcd, 0xef, 0x01}
	s := ShortRoot(root)
	if s != "0xabcdef01" {
		t.Fatalf("expected 0xabcdef01, got %s", s)
	}
}
