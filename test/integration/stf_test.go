package integration

import (
	"testing"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/types"
)

func makeTestValidators(n uint64) []*types.Validator {
	validators := make([]*types.Validator, n)
	for i := uint64(0); i < n; i++ {
		validators[i] = &types.Validator{Index: i}
	}
	return validators
}

func TestGenesisToBlock1StateTransition(t *testing.T) {
	state := statetransition.GenerateGenesis(1000, makeTestValidators(5))

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
		Body:          &types.BlockBody{Attestations: []*types.Attestation{}},
	}

	postState, err := statetransition.ProcessBlock(advanced, block)
	if err != nil {
		t.Fatalf("process_block: %v", err)
	}

	// Compute state root and set it on the block.
	stateRoot, _ := postState.HashTreeRoot()
	block.StateRoot = stateRoot

	// Full state transition.
	result, err := statetransition.StateTransition(state, block)
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
	state := statetransition.GenerateGenesis(1000, makeTestValidators(numValidators))

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
			Body:          &types.BlockBody{Attestations: []*types.Attestation{}},
		}

		postState, err := statetransition.ProcessBlock(advanced, block)
		if err != nil {
			t.Fatalf("slot %d process_block: %v", slot, err)
		}

		stateRoot, _ := postState.HashTreeRoot()
		block.StateRoot = stateRoot

		state, err = statetransition.StateTransition(state, block)
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
	state := statetransition.GenerateGenesis(1000, makeTestValidators(3))

	// Do one block transition.
	advanced, _ := statetransition.ProcessSlots(state, 1)
	parentRoot, _ := advanced.LatestBlockHeader.HashTreeRoot()
	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1,
		ParentRoot:    parentRoot,
		StateRoot:     types.ZeroHash,
		Body:          &types.BlockBody{Attestations: []*types.Attestation{}},
	}
	postState, _ := statetransition.ProcessBlock(advanced, block)
	stateRoot, _ := postState.HashTreeRoot()
	block.StateRoot = stateRoot

	result, err := statetransition.StateTransition(state, block)
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

// produceBlock is a helper that builds a valid block for the given slot,
// runs the full state transition, and returns the new state.
func produceBlock(t *testing.T, state *types.State, slot uint64, attestations []*types.Attestation) *types.State {
	t.Helper()
	numValidators := uint64(len(state.Validators))
	proposer := slot % numValidators

	advanced, err := statetransition.ProcessSlots(state, slot)
	if err != nil {
		t.Fatalf("process_slots(%d): %v", slot, err)
	}
	parentRoot, _ := advanced.LatestBlockHeader.HashTreeRoot()

	block := &types.Block{
		Slot:          slot,
		ProposerIndex: proposer,
		ParentRoot:    parentRoot,
		StateRoot:     types.ZeroHash,
		Body:          &types.BlockBody{Attestations: attestations},
	}
	postState, err := statetransition.ProcessBlock(advanced, block)
	if err != nil {
		t.Fatalf("process_block(%d): %v", slot, err)
	}
	stateRoot, _ := postState.HashTreeRoot()
	block.StateRoot = stateRoot

	result, err := statetransition.StateTransition(state, block)
	if err != nil {
		t.Fatalf("state_transition(%d): %v", slot, err)
	}
	return result
}

func TestBlocksWithAttestationsJustifyAndFinalize(t *testing.T) {
	numValidators := uint64(5)
	needed := (2*numValidators + 2) / 3 // 4 out of 5 for supermajority

	state := statetransition.GenerateGenesis(1000, makeTestValidators(numValidators))

	// Produce blocks for slots 1-3 without attestations to build history.
	for slot := uint64(1); slot <= 3; slot++ {
		state = produceBlock(t, state, slot, nil)
	}

	if len(state.HistoricalBlockHashes) != 3 {
		t.Fatalf("expected 3 historical hashes, got %d", len(state.HistoricalBlockHashes))
	}

	// Slot 4: include supermajority attestations justifying slot 1 from genesis (slot 0).
	source0 := &types.Checkpoint{Root: state.HistoricalBlockHashes[0], Slot: 0}
	target1 := &types.Checkpoint{Root: state.HistoricalBlockHashes[1], Slot: 1}
	var atts []*types.Attestation
	for i := uint64(0); i < needed; i++ {
		atts = append(atts, &types.Attestation{
			ValidatorID: i,
			Data: &types.AttestationData{
				Slot:   1,
				Head:   target1,
				Target: target1,
				Source: source0,
			},
		})
	}
	state = produceBlock(t, state, 4, atts)

	if state.LatestJustified.Slot != 1 {
		t.Fatalf("after slot 4: latest justified = %d, want 1", state.LatestJustified.Slot)
	}

	// Slot 5: include supermajority attestations justifying slot 2 from slot 1.
	// Consecutive justification (1→2) should finalize slot 1.
	source1 := &types.Checkpoint{Root: state.HistoricalBlockHashes[1], Slot: 1}
	target2 := &types.Checkpoint{Root: state.HistoricalBlockHashes[2], Slot: 2}
	atts = nil
	for i := uint64(0); i < needed; i++ {
		atts = append(atts, &types.Attestation{
			ValidatorID: i,
			Data: &types.AttestationData{
				Slot:   2,
				Head:   target2,
				Target: target2,
				Source: source1,
			},
		})
	}
	state = produceBlock(t, state, 5, atts)

	if state.LatestJustified.Slot != 2 {
		t.Fatalf("after slot 5: latest justified = %d, want 2", state.LatestJustified.Slot)
	}
	if state.LatestFinalized.Slot != 1 {
		t.Fatalf("after slot 5: latest finalized = %d, want 1", state.LatestFinalized.Slot)
	}

	// Verify state SSZ round-trip is stable after justification/finalization.
	data, err := state.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}
	decoded := new(types.State)
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ: %v", err)
	}
	root1, _ := state.HashTreeRoot()
	root2, _ := decoded.HashTreeRoot()
	if root1 != root2 {
		t.Errorf("state root changed after SSZ round-trip post-finalization: %x != %x", root1, root2)
	}
}

func TestGenesisWithPubkeysProducesStableRoot(t *testing.T) {
	// Build validators with distinct 52-byte XMSS pubkeys.
	validators := make([]*types.Validator, 5)
	for i := range validators {
		var pubkey [52]byte
		for j := range pubkey {
			pubkey[j] = byte(i*52 + j + 1)
		}
		validators[i] = &types.Validator{Pubkey: pubkey, Index: uint64(i)}
	}

	state := statetransition.GenerateGenesis(1000, validators)

	// Verify validator pubkeys are preserved.
	if len(state.Validators) != 5 {
		t.Fatalf("expected 5 validators, got %d", len(state.Validators))
	}
	for i, v := range state.Validators {
		if v.Pubkey != validators[i].Pubkey {
			t.Fatalf("validator %d pubkey mismatch", i)
		}
	}

	// Hash root should be deterministic and stable after SSZ round-trip.
	root1, _ := state.HashTreeRoot()
	data, err := state.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}
	decoded := new(types.State)
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ: %v", err)
	}
	root2, _ := decoded.HashTreeRoot()
	if root1 != root2 {
		t.Errorf("genesis root with pubkeys changed after SSZ round-trip: %x != %x", root1, root2)
	}

	// Pubkeys should survive the round-trip.
	for i, v := range decoded.Validators {
		if v.Pubkey != validators[i].Pubkey {
			t.Fatalf("validator %d pubkey mismatch after SSZ round-trip", i)
		}
	}
}
