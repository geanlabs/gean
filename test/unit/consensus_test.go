package unit

import (
	"testing"

	"github.com/devylongs/gean/chain/statetransition"
	"github.com/devylongs/gean/types"
)

func TestGenerateGenesis(t *testing.T) {
	state := statetransition.GenerateGenesis(1000, 30)

	if state.Slot != 0 {
		t.Errorf("genesis slot = %d, want 0", state.Slot)
	}
	if state.Config.NumValidators != 30 {
		t.Errorf("num_validators = %d, want 30", state.Config.NumValidators)
	}
	if state.Config.GenesisTime != 1000 {
		t.Errorf("genesis_time = %d, want 1000", state.Config.GenesisTime)
	}
	if state.LatestBlockHeader.Slot != 0 {
		t.Errorf("latest header slot = %d, want 0", state.LatestBlockHeader.Slot)
	}
	if state.LatestJustified.Root != types.ZeroHash {
		t.Error("genesis justified root should be zero")
	}
	if state.LatestFinalized.Root != types.ZeroHash {
		t.Error("genesis finalized root should be zero")
	}

	// Genesis state should have a valid hash tree root.
	root, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash tree root: %v", err)
	}
	if root == [32]byte{} {
		t.Fatal("genesis state root should not be zero")
	}
}

func TestIsProposer(t *testing.T) {
	// Round-robin: slot % numValidators == validatorIndex
	if !statetransition.IsProposer(0, 0, 30) {
		t.Error("validator 0 should be proposer for slot 0")
	}
	if !statetransition.IsProposer(1, 1, 30) {
		t.Error("validator 1 should be proposer for slot 1")
	}
	if !statetransition.IsProposer(0, 30, 30) {
		t.Error("validator 0 should be proposer for slot 30")
	}
	if statetransition.IsProposer(1, 0, 30) {
		t.Error("validator 1 should NOT be proposer for slot 0")
	}
}

func TestIsProposerPanicsOnZeroValidators(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic when numValidators is zero")
		}
	}()
	_ = statetransition.IsProposer(0, 0, 0)
}

func TestProcessSlots(t *testing.T) {
	state := statetransition.GenerateGenesis(1000, 5)

	// Process slots from 0 to 3.
	newState, err := statetransition.ProcessSlots(state, 3)
	if err != nil {
		t.Fatalf("process_slots: %v", err)
	}
	if newState.Slot != 3 {
		t.Errorf("slot = %d, want 3", newState.Slot)
	}
}

func TestProcessSlotsErrorOnPastSlot(t *testing.T) {
	state := statetransition.GenerateGenesis(1000, 5)
	state.Slot = 5

	_, err := statetransition.ProcessSlots(state, 3)
	if err == nil {
		t.Error("expected error for past slot")
	}
}

func TestProcessBlockHeader(t *testing.T) {
	state := statetransition.GenerateGenesis(1000, 5)

	// Advance to slot 1.
	state, err := statetransition.ProcessSlots(state, 1)
	if err != nil {
		t.Fatal(err)
	}

	// Build block for slot 1 (validator 1 is proposer since 1 % 5 == 1).
	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()
	emptyBody := &types.BlockBody{Attestations: []*types.SignedVote{}}
	bodyRoot, _ := emptyBody.HashTreeRoot()
	_ = bodyRoot

	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1,
		ParentRoot:    parentRoot,
		StateRoot:     types.ZeroHash,
		Body:          emptyBody,
	}

	newState, err := statetransition.ProcessBlockHeader(state, block)
	if err != nil {
		t.Fatalf("process_block_header: %v", err)
	}

	// First block after genesis should mark genesis as justified/finalized.
	if newState.LatestJustified.Root == types.ZeroHash {
		t.Error("genesis should be marked as justified after first block")
	}
	if newState.LatestFinalized.Root == types.ZeroHash {
		t.Error("genesis should be marked as finalized after first block")
	}

	// New header should have slot 1.
	if newState.LatestBlockHeader.Slot != 1 {
		t.Errorf("latest header slot = %d, want 1", newState.LatestBlockHeader.Slot)
	}

	// Historical hashes should have one entry (genesis block hash).
	if len(newState.HistoricalBlockHashes) != 1 {
		t.Errorf("historical hashes len = %d, want 1", len(newState.HistoricalBlockHashes))
	}
}

func TestProcessBlockHeaderWrongProposer(t *testing.T) {
	state := statetransition.GenerateGenesis(1000, 5)
	state, _ = statetransition.ProcessSlots(state, 1)
	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()

	block := &types.Block{
		Slot:          1,
		ProposerIndex: 0, // Wrong! Should be 1 for slot 1 with 5 validators.
		ParentRoot:    parentRoot,
		StateRoot:     types.ZeroHash,
		Body:          &types.BlockBody{Attestations: []*types.SignedVote{}},
	}

	_, err := statetransition.ProcessBlockHeader(state, block)
	if err == nil {
		t.Error("expected error for wrong proposer")
	}
}

func TestProcessBlockHeaderWrongParent(t *testing.T) {
	state := statetransition.GenerateGenesis(1000, 5)
	state, _ = statetransition.ProcessSlots(state, 1)

	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1,
		ParentRoot:    [32]byte{0xff}, // Wrong parent.
		StateRoot:     types.ZeroHash,
		Body:          &types.BlockBody{Attestations: []*types.SignedVote{}},
	}

	_, err := statetransition.ProcessBlockHeader(state, block)
	if err == nil {
		t.Error("expected error for wrong parent root")
	}
}

func TestProcessBlock(t *testing.T) {
	state := statetransition.GenerateGenesis(1000, 5)
	state, _ = statetransition.ProcessSlots(state, 1)
	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()

	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1,
		ParentRoot:    parentRoot,
		StateRoot:     types.ZeroHash,
		Body:          &types.BlockBody{Attestations: []*types.SignedVote{}},
	}

	newState, err := statetransition.ProcessBlock(state, block)
	if err != nil {
		t.Fatalf("process_block: %v", err)
	}

	if newState.LatestBlockHeader.Slot != 1 {
		t.Errorf("latest header slot = %d, want 1", newState.LatestBlockHeader.Slot)
	}
}

func TestEmptySlotGaps(t *testing.T) {
	state := statetransition.GenerateGenesis(1000, 5)

	// Advance to slot 3 (proposer is validator 3).
	state, _ = statetransition.ProcessSlots(state, 3)
	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()

	block := &types.Block{
		Slot:          3,
		ProposerIndex: 3, // 3 % 5 == 3
		ParentRoot:    parentRoot,
		StateRoot:     types.ZeroHash,
		Body:          &types.BlockBody{Attestations: []*types.SignedVote{}},
	}

	newState, err := statetransition.ProcessBlockHeader(state, block)
	if err != nil {
		t.Fatalf("process_block_header: %v", err)
	}

	// Should have 3 entries: genesis hash + 2 empty slot zeros.
	// (genesis block at slot 0, empty slots 1 and 2)
	if len(newState.HistoricalBlockHashes) != 3 {
		t.Errorf("historical hashes len = %d, want 3", len(newState.HistoricalBlockHashes))
	}
}
