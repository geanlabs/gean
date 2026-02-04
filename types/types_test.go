package types

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
)

func TestRoot_IsZero(t *testing.T) {
	tests := []struct {
		name string
		root Root
		want bool
	}{
		{"zero root", Root{}, true},
		{"non-zero first byte", Root{1}, false},
		{"non-zero last byte", func() Root { var r Root; r[31] = 1; return r }(), false},
		{"all ones", func() Root { var r Root; for i := range r { r[i] = 0xff }; return r }(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.root.IsZero(); got != tt.want {
				t.Errorf("Root.IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSlot_IsJustifiableAfter(t *testing.T) {
	tests := []struct {
		name          string
		slot          Slot
		finalizedSlot Slot
		want          bool
	}{
		// Immediate range (delta <= 5)
		{"delta 0", 10, 10, true},
		{"delta 1", 11, 10, true},
		{"delta 5", 15, 10, true},
		// Perfect squares
		{"delta 9 (3^2)", 19, 10, true},
		{"delta 16 (4^2)", 26, 10, true},
		{"delta 25 (5^2)", 35, 10, true},

		// Pronic numbers (n*(n+1))
		{"delta 6 (2*3)", 16, 10, true},
		{"delta 12 (3*4)", 22, 10, true},
		{"delta 20 (4*5)", 30, 10, true},

		// Non-justifiable
		{"delta 7", 17, 10, false},
		{"delta 8", 18, 10, false},
		{"delta 10", 20, 10, false},

		// Slot before finalized
		{"slot before finalized", 5, 10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.slot.IsJustifiableAfter(tt.finalizedSlot); got != tt.want {
				t.Errorf("Slot(%d).IsJustifiableAfter(%d) = %v, want %v",
					tt.slot, tt.finalizedSlot, got, tt.want)
			}
		})
	}
}

func TestCheckpoint_HashTreeRoot(t *testing.T) {
	cp := Checkpoint{
		Root: Root{1, 2, 3},
		Slot: 100,
	}

	root, err := cp.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot() error = %v", err)
	}

	// Hash should be non-zero
	if root == (Root{}) {
		t.Error("HashTreeRoot() returned zero root")
	}

	// Same input should produce same hash
	root2, _ := cp.HashTreeRoot()
	if root != root2 {
		t.Error("HashTreeRoot() not deterministic")
	}

	// Different input should produce different hash
	cp2 := Checkpoint{Root: Root{4, 5, 6}, Slot: 200}
	root3, _ := cp2.HashTreeRoot()
	if root == root3 {
		t.Error("Different checkpoints should have different hashes")
	}
}

func TestBlock_HashTreeRoot(t *testing.T) {
	block := Block{
		Slot:          10,
		ProposerIndex: 5,
		ParentRoot:    Root{1, 2, 3},
		StateRoot:     Root{4, 5, 6},
		Body:          BlockBody{},
	}

	root, err := block.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot() error = %v", err)
	}

	if root == (Root{}) {
		t.Error("HashTreeRoot() returned zero root")
	}

	// Deterministic
	root2, _ := block.HashTreeRoot()
	if root != root2 {
		t.Error("HashTreeRoot() not deterministic")
	}
}

func TestState_HashTreeRoot(t *testing.T) {
	// Use bitfield library to create valid SSZ bitlists
	justifiedSlots := bitfield.NewBitlist(1)
	justificationValidators := bitfield.NewBitlist(1)

	state := State{
		Slot: 10,
		Config: Config{
			NumValidators: 8,
			GenesisTime:   1000000000,
		},
		LatestBlockHeader: BlockHeader{
			Slot: 9,
		},
		LatestJustified:          Checkpoint{Slot: 5},
		LatestFinalized:          Checkpoint{Slot: 3},
		HistoricalBlockHashes:    []Root{{1}, {2}, {3}},
		JustifiedSlots:           justifiedSlots,
		JustificationValidators:  justificationValidators,
	}

	root, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot() error = %v", err)
	}

	if root == (Root{}) {
		t.Error("HashTreeRoot() returned zero root")
	}
}

func TestConstants(t *testing.T) {
	// Verify constants are set correctly
	if SecondsPerSlot != 4 {
		t.Errorf("SecondsPerSlot = %d, want 4", SecondsPerSlot)
	}
	if IntervalsPerSlot != 4 {
		t.Errorf("IntervalsPerSlot = %d, want 4", IntervalsPerSlot)
	}
	if SecondsPerInterval != 1 {
		t.Errorf("SecondsPerInterval = %d, want 1", SecondsPerInterval)
	}
}
