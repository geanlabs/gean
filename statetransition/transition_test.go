package statetransition

import (
	"testing"

	"github.com/geanlabs/gean/types"
)

// makeGenesisState creates a minimal genesis state with n validators.
func makeGenesisState(n int) *types.State {
	validators := make([]*types.Validator, n)
	for i := 0; i < n; i++ {
		var pubkey [types.PubkeySize]byte
		pubkey[0] = byte(i + 1)
		validators[i] = &types.Validator{Pubkey: pubkey, Index: uint64(i)}
	}
	return &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     0,
		LatestBlockHeader:        &types.BlockHeader{Slot: 0},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		HistoricalBlockHashes:    nil,
		JustifiedSlots:           types.NewBitlistSSZ(0),
		Validators:               validators,
		JustificationsRoots:      nil,
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
}

func TestProcessSlotsAdvancesSlot(t *testing.T) {
	state := makeGenesisState(3)
	if err := ProcessSlots(state, 5); err != nil {
		t.Fatal(err)
	}
	if state.Slot != 5 {
		t.Fatalf("expected slot 5, got %d", state.Slot)
	}
}

func TestProcessSlotsCachesStateRoot(t *testing.T) {
	state := makeGenesisState(3)
	if state.LatestBlockHeader.StateRoot != types.ZeroRoot {
		t.Fatal("genesis header should have zero state root")
	}
	if err := ProcessSlots(state, 1); err != nil {
		t.Fatal(err)
	}
	if state.LatestBlockHeader.StateRoot == types.ZeroRoot {
		t.Fatal("state root should be cached after process_slots")
	}
}

func TestProcessSlotsRejectsOlderSlot(t *testing.T) {
	state := makeGenesisState(3)
	state.Slot = 10
	err := ProcessSlots(state, 5)
	if err == nil {
		t.Fatal("should reject target slot < current slot")
	}
	if _, ok := err.(*StateSlotIsNewerError); !ok {
		t.Fatalf("expected StateSlotIsNewerError, got %T", err)
	}
}

func TestProcessBlockHeaderValidatesSlot(t *testing.T) {
	state := makeGenesisState(3)
	state.Slot = 1

	block := &types.Block{
		Slot:          2, // doesn't match state.Slot
		ProposerIndex: 0,
		Body:          &types.BlockBody{},
	}
	err := ProcessBlockHeader(state, block)
	if err == nil {
		t.Fatal("should reject slot mismatch")
	}
}

func TestProcessBlockHeaderValidatesProposer(t *testing.T) {
	state := makeGenesisState(3)
	// Advance state to slot 1 via process_slots.
	ProcessSlots(state, 1)

	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()
	block := &types.Block{
		Slot:          1,
		ProposerIndex: 2, // wrong: slot 1 % 3 = 1
		ParentRoot:    parentRoot,
		Body:          &types.BlockBody{},
	}
	err := ProcessBlockHeader(state, block)
	if err == nil {
		t.Fatal("should reject wrong proposer")
	}
	if _, ok := err.(*InvalidProposerError); !ok {
		t.Fatalf("expected InvalidProposerError, got %T: %v", err, err)
	}
}

func TestProcessBlockHeaderValidatesParentRoot(t *testing.T) {
	state := makeGenesisState(3)
	ProcessSlots(state, 1)

	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1,              // correct: 1 % 3 = 1
		ParentRoot:    [32]byte{0xff}, // wrong parent root
		Body:          &types.BlockBody{},
	}
	err := ProcessBlockHeader(state, block)
	if err == nil {
		t.Fatal("should reject wrong parent root")
	}
	if _, ok := err.(*InvalidParentError); !ok {
		t.Fatalf("expected InvalidParentError, got %T: %v", err, err)
	}
}

func TestProcessBlockHeaderUpdatesState(t *testing.T) {
	state := makeGenesisState(3)
	ProcessSlots(state, 1)

	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()
	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1, // 1 % 3 = 1
		ParentRoot:    parentRoot,
		Body:          &types.BlockBody{},
	}
	if err := ProcessBlockHeader(state, block); err != nil {
		t.Fatal(err)
	}

	// Block header should be updated.
	if state.LatestBlockHeader.Slot != 1 {
		t.Fatalf("expected header slot 1, got %d", state.LatestBlockHeader.Slot)
	}
	if state.LatestBlockHeader.StateRoot != types.ZeroRoot {
		t.Fatal("new header should have zero state root")
	}

	// Historical block hashes should have parent root.
	if len(state.HistoricalBlockHashes) != 1 {
		t.Fatalf("expected 1 historical hash, got %d", len(state.HistoricalBlockHashes))
	}
}

func TestProcessBlockHeaderSkippedSlots(t *testing.T) {
	state := makeGenesisState(3)
	ProcessSlots(state, 4)

	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()
	block := &types.Block{
		Slot:          4,
		ProposerIndex: 1, // 4 % 3 = 1
		ParentRoot:    parentRoot,
		Body:          &types.BlockBody{},
	}
	if err := ProcessBlockHeader(state, block); err != nil {
		t.Fatal(err)
	}

	// Parent was at slot 0, block at slot 4: 1 parent + 3 empty = 4 entries.
	if len(state.HistoricalBlockHashes) != 4 {
		t.Fatalf("expected 4 historical hashes, got %d", len(state.HistoricalBlockHashes))
	}

	// First should be parent root, rest should be zero.
	var zeroHash [32]byte
	var first [32]byte
	copy(first[:], state.HistoricalBlockHashes[0])
	if first == zeroHash {
		t.Fatal("first entry should be parent root, not zero")
	}
	for i := 1; i < 4; i++ {
		var h [32]byte
		copy(h[:], state.HistoricalBlockHashes[i])
		if h != zeroHash {
			t.Fatalf("entry %d should be zero for skipped slot", i)
		}
	}
}
