package integration

import (
	"testing"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/types"
)

func TestGenesisToBlock1StateTransition(t *testing.T) {
	state := statetransition.GenerateGenesis(1000, 5)

	// Advance to slot 1 and produce a block.
	advanced, err := statetransition.ProcessSlots(state, 1)
	if err != nil {
		t.Fatal(err)
	}

	parentRoot, _ := advanced.LatestBlockHeader.HashTreeRoot()
	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1, // 1 % 5 == 1
		ParentRoot:    parentRoot,
		StateRoot:     types.ZeroHash,
		Body:          &types.BlockBody{Attestations: []*types.SignedVote{}},
	}

	postState, err := statetransition.ProcessBlock(advanced, block)
	if err != nil {
		t.Fatalf("process_block: %v", err)
	}

	// Compute state root and set it on the block.
	stateRoot, _ := postState.HashTreeRoot()
	block.StateRoot = stateRoot

	// Full state transition with the signed block.
	signed := &types.SignedBlock{Message: block, Signature: types.ZeroHash}
	result, err := statetransition.StateTransition(state, signed)
	if err != nil {
		t.Fatalf("state_transition: %v", err)
	}

	if result.LatestBlockHeader.Slot != 1 {
		t.Errorf("result header slot = %d, want 1", result.LatestBlockHeader.Slot)
	}

	// Genesis should be marked as justified and finalized.
	if result.LatestJustified.Root == types.ZeroHash {
		t.Error("genesis should be justified after first block")
	}
}

func TestMultipleBlocksAdvanceHead(t *testing.T) {
	numValidators := uint64(5)
	state := statetransition.GenerateGenesis(1000, numValidators)

	// Produce blocks for slots 1 through 5.
	for slot := uint64(1); slot <= 5; slot++ {
		proposer := slot % numValidators
		advanced, err := statetransition.ProcessSlots(state, slot)
		if err != nil {
			t.Fatalf("slot %d process_slots: %v", slot, err)
		}

		parentRoot, _ := advanced.LatestBlockHeader.HashTreeRoot()
		block := &types.Block{
			Slot:          slot,
			ProposerIndex: proposer,
			ParentRoot:    parentRoot,
			StateRoot:     types.ZeroHash,
			Body:          &types.BlockBody{Attestations: []*types.SignedVote{}},
		}

		postState, err := statetransition.ProcessBlock(advanced, block)
		if err != nil {
			t.Fatalf("slot %d process_block: %v", slot, err)
		}

		stateRoot, _ := postState.HashTreeRoot()
		block.StateRoot = stateRoot

		signed := &types.SignedBlock{Message: block, Signature: types.ZeroHash}
		state, err = statetransition.StateTransition(state, signed)
		if err != nil {
			t.Fatalf("slot %d state_transition: %v", slot, err)
		}
	}

	if state.LatestBlockHeader.Slot != 5 {
		t.Errorf("final header slot = %d, want 5", state.LatestBlockHeader.Slot)
	}

	// Should have 5 historical block hashes.
	if len(state.HistoricalBlockHashes) != 5 {
		t.Errorf("historical hashes len = %d, want 5", len(state.HistoricalBlockHashes))
	}
}

func TestSSZRoundTripAfterStateTransition(t *testing.T) {
	state := statetransition.GenerateGenesis(1000, 3)

	// Do one block transition.
	advanced, _ := statetransition.ProcessSlots(state, 1)
	parentRoot, _ := advanced.LatestBlockHeader.HashTreeRoot()
	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1,
		ParentRoot:    parentRoot,
		StateRoot:     types.ZeroHash,
		Body:          &types.BlockBody{Attestations: []*types.SignedVote{}},
	}
	postState, _ := statetransition.ProcessBlock(advanced, block)
	stateRoot, _ := postState.HashTreeRoot()
	block.StateRoot = stateRoot

	signed := &types.SignedBlock{Message: block, Signature: types.ZeroHash}
	result, err := statetransition.StateTransition(state, signed)
	if err != nil {
		t.Fatal(err)
	}

	// SSZ round-trip the resulting state.
	data, err := result.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := new(types.State)
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Hash roots should match.
	originalRoot, _ := result.HashTreeRoot()
	decodedRoot, _ := decoded.HashTreeRoot()
	if originalRoot != decodedRoot {
		t.Errorf("state root mismatch after SSZ round-trip: %x != %x", originalRoot, decodedRoot)
	}
}
