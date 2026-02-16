package unit

import (
	"testing"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/types"
)

// TestCopyStateIndependence verifies that copyState (called internally by
// ProcessSlots/ProcessBlock) produces a fully independent copy. Mutating any
// field in the copy must not affect the original.
func TestCopyStateIndependence(t *testing.T) {
	validators := make([]*types.Validator, 5)
	for i := range validators {
		var pk [52]byte
		pk[0] = byte(i + 1)
		validators[i] = &types.Validator{Pubkey: pk, Index: uint64(i)}
	}
	original := statetransition.GenerateGenesis(1000, validators)

	// Snapshot original values.
	origRoot, _ := original.HashTreeRoot()

	// ProcessSlots calls copyState internally.
	copied, err := statetransition.ProcessSlots(original, 1)
	if err != nil {
		t.Fatal(err)
	}

	// Mutate every field in the copy.
	copied.Config.GenesisTime = 9999
	copied.LatestBlockHeader.Slot = 999
	copied.LatestJustified.Slot = 888
	copied.LatestFinalized.Slot = 777
	copied.Validators[0].Pubkey = [52]byte{0xff}
	copied.Validators = append(copied.Validators, &types.Validator{Index: 99})
	copied.HistoricalBlockHashes = append(copied.HistoricalBlockHashes, [32]byte{0xaa})
	copied.JustifiedSlots = append(copied.JustifiedSlots, 0xff)
	copied.JustificationsRoots = append(copied.JustificationsRoots, [32]byte{0xbb})
	copied.JustificationsValidators = append(copied.JustificationsValidators, 0xcc)

	// Original must be completely unaffected.
	afterRoot, _ := original.HashTreeRoot()
	if origRoot != afterRoot {
		t.Fatalf("original state root changed: %x -> %x", origRoot, afterRoot)
	}
	if original.Config.GenesisTime != 1000 {
		t.Error("Config.GenesisTime mutated")
	}
	if len(original.Validators) != 5 {
		t.Errorf("validator count changed: got %d", len(original.Validators))
	}
	if original.Validators[0].Pubkey[0] != 1 {
		t.Error("validator pubkey mutated")
	}
}

// TestStateTransitionDoesNotMutateInput verifies that a full state transition
// does not modify the input state.
func TestStateTransitionDoesNotMutateInput(t *testing.T) {
	original := statetransition.GenerateGenesis(1000, makeTestValidators(5))
	origRoot, _ := original.HashTreeRoot()

	// Produce a valid block and run full state transition.
	advanced, err := statetransition.ProcessSlots(original, 1)
	if err != nil {
		t.Fatal(err)
	}
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

	if _, err := statetransition.StateTransition(original, block); err != nil {
		t.Fatal(err)
	}

	afterRoot, _ := original.HashTreeRoot()
	if origRoot != afterRoot {
		t.Errorf("original state root changed after StateTransition: %x -> %x", origRoot, afterRoot)
	}
}

// TestProcessAttestationsDoesNotMutateInput verifies that ProcessAttestations
// does not mutate the input state.
func TestProcessAttestationsDoesNotMutateInput(t *testing.T) {
	state := statetransition.GenerateGenesis(1000, makeTestValidators(5))

	// Build history so attestations can reference slots.
	for slot := uint64(1); slot <= 3; slot++ {
		advanced, _ := statetransition.ProcessSlots(state, slot)
		parentRoot, _ := advanced.LatestBlockHeader.HashTreeRoot()
		block := &types.Block{
			Slot:          slot,
			ProposerIndex: slot % 5,
			ParentRoot:    parentRoot,
			StateRoot:     types.ZeroHash,
			Body:          &types.BlockBody{Attestations: []*types.Attestation{}},
		}
		postState, _ := statetransition.ProcessBlock(advanced, block)
		stateRoot, _ := postState.HashTreeRoot()
		block.StateRoot = stateRoot
		var err error
		state, err = statetransition.StateTransition(state, block)
		if err != nil {
			t.Fatal(err)
		}
	}

	origRoot, _ := state.HashTreeRoot()

	// Supermajority attestations justifying slot 1 from genesis.
	source0 := &types.Checkpoint{Root: state.HistoricalBlockHashes[0], Slot: 0}
	target1 := &types.Checkpoint{Root: state.HistoricalBlockHashes[1], Slot: 1}
	var atts []*types.Attestation
	for i := uint64(0); i < 4; i++ {
		atts = append(atts, &types.Attestation{
			ValidatorID: i,
			Data:        &types.AttestationData{Slot: 1, Head: target1, Target: target1, Source: source0},
		})
	}

	_ = statetransition.ProcessAttestations(state, atts)

	afterRoot, _ := state.HashTreeRoot()
	if origRoot != afterRoot {
		t.Errorf("state root changed after ProcessAttestations: %x -> %x", origRoot, afterRoot)
	}
}
