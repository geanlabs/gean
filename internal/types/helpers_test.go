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
		{5, 0, 5, true},
		{6, 1, 5, true},
		{0, 1, 5, false},
		{1, 0, 5, false},
		{0, 0, 0, false},
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
	if ProposerIndex(7, 0) != 0 {
		t.Fatal("expected proposer 0 with no validators")
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

func TestStateNumValidatorsNilSafe(t *testing.T) {
	var state *State
	if state.NumValidators() != 0 {
		t.Fatal("nil state should have zero validators")
	}
}

func TestStateCloneDeepCopiesSSZFields(t *testing.T) {
	root := [32]byte{0x01}
	state := &State{
		Config:                   &ChainConfig{GenesisTime: 1},
		Slot:                     4,
		LatestBlockHeader:        &BlockHeader{Slot: 3},
		LatestJustified:          &Checkpoint{Slot: 2},
		LatestFinalized:          &Checkpoint{Slot: 1},
		HistoricalBlockHashes:    [][]byte{root[:]},
		JustifiedSlots:           NewBitlistSSZ(2),
		Validators:               []*Validator{{Index: 1}},
		JustificationsValidators: NewBitlistSSZ(0),
	}

	clone, err := state.Clone()
	if err != nil {
		t.Fatalf("clone state: %v", err)
	}
	if clone == state || clone.LatestBlockHeader == state.LatestBlockHeader || clone.Validators[0] == state.Validators[0] {
		t.Fatal("clone shares pointer fields with original")
	}

	clone.Slot = 9
	clone.LatestBlockHeader.Slot = 8
	clone.Validators[0].Index = 7
	if state.Slot != 4 || state.LatestBlockHeader.Slot != 3 || state.Validators[0].Index != 1 {
		t.Fatal("mutating clone changed original state")
	}
}
