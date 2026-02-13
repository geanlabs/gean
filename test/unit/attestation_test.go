package unit

import (
	"bytes"
	"testing"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/types"
)

// buildChainState produces consecutive blocks from genesis through targetSlot
// (inclusive), returning the final state and a map of slot -> block hash.
func buildChainState(t *testing.T, numValidators, targetSlot uint64) (*types.State, map[uint64][32]byte) {
	t.Helper()

	state := statetransition.GenerateGenesis(1000, numValidators)
	blockHashes := make(map[uint64][32]byte)

	emptyBody := &types.BlockBody{Attestations: []*types.SignedVote{}}
	genesisBlock := &types.Block{
		Slot:          0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash,
		Body:          emptyBody,
		ProposerIndex: 0,
	}
	stateRoot, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("genesis state root: %v", err)
	}
	genesisBlock.StateRoot = stateRoot
	genesisHash, err := genesisBlock.HashTreeRoot()
	if err != nil {
		t.Fatalf("genesis block root: %v", err)
	}
	blockHashes[0] = genesisHash

	for slot := uint64(1); slot <= targetSlot; slot++ {
		proposer := slot % numValidators
		advanced, err := statetransition.ProcessSlots(state, slot)
		if err != nil {
			t.Fatalf("process slots(%d): %v", slot, err)
		}
		parentRoot, err := advanced.LatestBlockHeader.HashTreeRoot()
		if err != nil {
			t.Fatalf("parent root(%d): %v", slot, err)
		}

		block := &types.Block{
			Slot:          slot,
			ProposerIndex: proposer,
			ParentRoot:    parentRoot,
			StateRoot:     types.ZeroHash,
			Body:          &types.BlockBody{Attestations: []*types.SignedVote{}},
		}
		postState, err := statetransition.ProcessBlock(advanced, block)
		if err != nil {
			t.Fatalf("process block(%d): %v", slot, err)
		}
		sr, err := postState.HashTreeRoot()
		if err != nil {
			t.Fatalf("post state root(%d): %v", slot, err)
		}
		block.StateRoot = sr

		signed := &types.SignedBlock{Message: block, Signature: types.ZeroHash}
		state, err = statetransition.StateTransition(state, signed)
		if err != nil {
			t.Fatalf("state transition(%d): %v", slot, err)
		}

		bh, err := block.HashTreeRoot()
		if err != nil {
			t.Fatalf("block hash(%d): %v", slot, err)
		}
		blockHashes[slot] = bh
	}

	return state, blockHashes
}

func makeVote(validatorID uint64, source, target *types.Checkpoint) *types.SignedVote {
	return &types.SignedVote{
		Data: &types.Vote{
			ValidatorID: validatorID,
			Slot:        target.Slot,
			Head:        target,
			Target:      target,
			Source:      source,
		},
		Signature: types.ZeroHash,
	}
}

func TestAttestationJustifiesTargetWhenSourceIsJustified(t *testing.T) {
	state, hashes := buildChainState(t, 5, 2)

	source := &types.Checkpoint{Root: hashes[0], Slot: 0} // justified by first block processing
	target := &types.Checkpoint{Root: hashes[1], Slot: 1}

	next := statetransition.ProcessAttestations(state, []*types.SignedVote{makeVote(0, source, target)})

	if next.LatestJustified.Slot != 1 {
		t.Fatalf("latest justified slot = %d, want 1", next.LatestJustified.Slot)
	}
}

func TestAttestationIgnoredWhenSourceNotJustified(t *testing.T) {
	state, hashes := buildChainState(t, 5, 2)

	// Slot 1 is not justified by default in this chain progression.
	source := &types.Checkpoint{Root: hashes[1], Slot: 1}
	target := &types.Checkpoint{Root: hashes[2], Slot: 2}

	next := statetransition.ProcessAttestations(state, []*types.SignedVote{makeVote(0, source, target)})

	if next.LatestJustified.Slot != state.LatestJustified.Slot {
		t.Fatalf("latest justified slot changed: got %d want %d", next.LatestJustified.Slot, state.LatestJustified.Slot)
	}
}

func TestAttestationIgnoredWhenSourceIsNotBeforeTarget(t *testing.T) {
	state, hashes := buildChainState(t, 5, 2)

	source := &types.Checkpoint{Root: hashes[1], Slot: 1}
	target := &types.Checkpoint{Root: hashes[1], Slot: 1}

	next := statetransition.ProcessAttestations(state, []*types.SignedVote{makeVote(0, source, target)})

	if next.LatestJustified.Slot != state.LatestJustified.Slot {
		t.Fatalf("latest justified slot changed: got %d want %d", next.LatestJustified.Slot, state.LatestJustified.Slot)
	}
}

func TestAlreadyJustifiedConsecutiveTargetCanFinalize(t *testing.T) {
	state, hashes := buildChainState(t, 5, 3)

	source0 := &types.Checkpoint{Root: hashes[0], Slot: 0}
	target1 := &types.Checkpoint{Root: hashes[1], Slot: 1}
	state = statetransition.ProcessAttestations(state, []*types.SignedVote{makeVote(0, source0, target1)})

	source1 := &types.Checkpoint{Root: hashes[1], Slot: 1}
	target2 := &types.Checkpoint{Root: hashes[2], Slot: 2}
	state = statetransition.ProcessAttestations(state, []*types.SignedVote{makeVote(1, source1, target2)})

	// Force latest_justified behind an already-justified target to exercise
	// the leanSpec "target already justified" finalization branch.
	state.LatestJustified = &types.Checkpoint{Root: hashes[1], Slot: 1}
	state.LatestFinalized = &types.Checkpoint{Root: hashes[0], Slot: 0}

	next := statetransition.ProcessAttestations(state, []*types.SignedVote{makeVote(2, source1, target2)})

	if next.LatestFinalized.Slot != 1 {
		t.Fatalf("latest finalized slot = %d, want 1", next.LatestFinalized.Slot)
	}
	if next.LatestJustified.Slot != 2 {
		t.Fatalf("latest justified slot = %d, want 2", next.LatestJustified.Slot)
	}
}

func TestAttestationsDoNotMutateJustificationTrackingLists(t *testing.T) {
	state, hashes := buildChainState(t, 5, 2)

	beforeRoots := append([][32]byte(nil), state.JustificationsRoots...)
	beforeVals := append([]byte(nil), state.JustificationsValidators...)

	source := &types.Checkpoint{Root: hashes[0], Slot: 0}
	target := &types.Checkpoint{Root: hashes[1], Slot: 1}
	next := statetransition.ProcessAttestations(state, []*types.SignedVote{makeVote(0, source, target)})

	if len(next.JustificationsRoots) != len(beforeRoots) {
		t.Fatalf("justifications_roots length changed: got %d want %d", len(next.JustificationsRoots), len(beforeRoots))
	}
	if !bytes.Equal(next.JustificationsValidators, beforeVals) {
		t.Fatal("justifications_validators changed unexpectedly")
	}
}
