package unit

import (
	"testing"

	"github.com/devylongs/gean/chain/forkchoice"
	"github.com/devylongs/gean/chain/statetransition"
	"github.com/devylongs/gean/types"
)

func buildForkChoiceWithBlocks(t *testing.T, numValidators, targetSlot uint64) (*forkchoice.Store, map[uint64][32]byte) {
	t.Helper()

	fc, state := makeGenesisFC(numValidators)
	blockHashes := map[uint64][32]byte{0: fc.Head}

	for slot := uint64(1); slot <= targetSlot; slot++ {
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
			ProposerIndex: slot % numValidators,
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
			t.Fatalf("post-state root(%d): %v", slot, err)
		}
		block.StateRoot = sr

		signed := &types.SignedBlock{Message: block, Signature: types.ZeroHash}
		state, err = statetransition.StateTransition(state, signed)
		if err != nil {
			t.Fatalf("state transition(%d): %v", slot, err)
		}

		if err := fc.ProcessBlock(block); err != nil {
			t.Fatalf("forkchoice process block(%d): %v", slot, err)
		}
		bh, err := block.HashTreeRoot()
		if err != nil {
			t.Fatalf("block hash(%d): %v", slot, err)
		}
		blockHashes[slot] = bh
	}

	return fc, blockHashes
}

func TestForkChoiceProcessAttestationValidGossip(t *testing.T) {
	fc, hashes := buildForkChoiceWithBlocks(t, 5, 2)
	fc.Time = 10 * types.IntervalsPerSlot // current slot far ahead of vote slot

	vote := &types.Vote{
		ValidatorID: 5,
		Slot:        2,
		Head:        &types.Checkpoint{Root: hashes[2], Slot: 2},
		Source:      &types.Checkpoint{Root: hashes[1], Slot: 1},
		Target:      &types.Checkpoint{Root: hashes[2], Slot: 2},
	}
	fc.ProcessAttestation(&types.SignedVote{Data: vote, Signature: types.ZeroHash})

	got, ok := fc.LatestNewVotes[5]
	if !ok {
		t.Fatal("expected validator vote in latest_new_votes")
	}
	if got.Slot != 2 || got.Root != hashes[2] {
		t.Fatalf("unexpected vote target: got slot=%d root=%x", got.Slot, got.Root)
	}
}

func TestForkChoiceProcessAttestationRejectsCheckpointSlotMismatch(t *testing.T) {
	fc, hashes := buildForkChoiceWithBlocks(t, 5, 2)
	fc.Time = 10 * types.IntervalsPerSlot

	vote := &types.Vote{
		ValidatorID: 1,
		Slot:        2,
		Head:        &types.Checkpoint{Root: hashes[2], Slot: 2},
		Source:      &types.Checkpoint{Root: hashes[1], Slot: 0}, // mismatch: block slot is 1
		Target:      &types.Checkpoint{Root: hashes[2], Slot: 2},
	}
	fc.ProcessAttestation(&types.SignedVote{Data: vote, Signature: types.ZeroHash})

	if len(fc.LatestNewVotes) != 0 {
		t.Fatalf("expected no new votes, got %d", len(fc.LatestNewVotes))
	}
}

func TestForkChoiceProcessAttestationRejectsTooFarFuture(t *testing.T) {
	fc, hashes := buildForkChoiceWithBlocks(t, 5, 2)
	fc.Time = 2 * types.IntervalsPerSlot // current slot = 2

	vote := &types.Vote{
		ValidatorID: 2,
		Slot:        4, // > currentSlot + 1
		Head:        &types.Checkpoint{Root: hashes[2], Slot: 2},
		Source:      &types.Checkpoint{Root: hashes[1], Slot: 1},
		Target:      &types.Checkpoint{Root: hashes[2], Slot: 2},
	}
	fc.ProcessAttestation(&types.SignedVote{Data: vote, Signature: types.ZeroHash})

	if len(fc.LatestNewVotes) != 0 {
		t.Fatalf("expected no new votes, got %d", len(fc.LatestNewVotes))
	}
}

func TestForkChoiceProcessAttestationRejectsFutureGossipVote(t *testing.T) {
	fc, hashes := buildForkChoiceWithBlocks(t, 5, 2)
	fc.Time = 2 * types.IntervalsPerSlot // current slot = 2

	vote := &types.Vote{
		ValidatorID: 3,
		Slot:        3, // <= currentSlot+1 but > currentSlot, should fail gossip check
		Head:        &types.Checkpoint{Root: hashes[2], Slot: 2},
		Source:      &types.Checkpoint{Root: hashes[1], Slot: 1},
		Target:      &types.Checkpoint{Root: hashes[2], Slot: 2},
	}
	fc.ProcessAttestation(&types.SignedVote{Data: vote, Signature: types.ZeroHash})

	if len(fc.LatestNewVotes) != 0 {
		t.Fatalf("expected no new votes, got %d", len(fc.LatestNewVotes))
	}
}
