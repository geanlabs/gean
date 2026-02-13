package unit

import (
	"testing"

	"github.com/geanlabs/gean/types"
)

func TestProduceBlockCreatesValidBlock(t *testing.T) {
	fc, _ := buildForkChoiceWithBlocks(t, 5, 3)

	// Slot 4, proposer = 4 % 5 = 4
	block, err := fc.ProduceBlock(4, 4)
	if err != nil {
		t.Fatalf("ProduceBlock: %v", err)
	}

	if block.Slot != 4 {
		t.Fatalf("block.Slot = %d, want 4", block.Slot)
	}
	if block.ProposerIndex != 4 {
		t.Fatalf("block.ProposerIndex = %d, want 4", block.ProposerIndex)
	}
	if block.StateRoot == types.ZeroHash {
		t.Fatal("block.StateRoot should not be zero")
	}

	// Block should be stored.
	blockHash, _ := block.HashTreeRoot()
	if _, ok := fc.Storage.GetBlock(blockHash); !ok {
		t.Fatal("produced block should be stored")
	}
	if _, ok := fc.Storage.GetState(blockHash); !ok {
		t.Fatal("produced block state should be stored")
	}
}

func TestProduceBlockRejectsWrongProposer(t *testing.T) {
	fc, _ := buildForkChoiceWithBlocks(t, 5, 3)

	// Slot 4 proposer is 4, not 0.
	_, err := fc.ProduceBlock(4, 0)
	if err == nil {
		t.Fatal("expected error for wrong proposer")
	}
}

func TestProduceBlockIncludesAttestations(t *testing.T) {
	fc, hashes := buildForkChoiceWithBlocks(t, 5, 3)

	// Add votes for slot 3 block.
	for i := uint64(0); i < 3; i++ {
		fc.LatestKnownVotes[i] = &types.Checkpoint{Root: hashes[3], Slot: 3}
	}

	block, err := fc.ProduceBlock(4, 4)
	if err != nil {
		t.Fatalf("ProduceBlock: %v", err)
	}

	if len(block.Body.Attestations) == 0 {
		t.Fatal("block should include attestations from known votes")
	}
}

func TestProduceAttestationVoteReturnsValidVote(t *testing.T) {
	fc, _ := buildForkChoiceWithBlocks(t, 5, 2)

	vote := fc.ProduceAttestationVote(3, 0)

	if vote.ValidatorID != 0 {
		t.Fatalf("vote.ValidatorID = %d, want 0", vote.ValidatorID)
	}
	if vote.Slot != 3 {
		t.Fatalf("vote.Slot = %d, want 3", vote.Slot)
	}
	if vote.Head == nil || vote.Target == nil || vote.Source == nil {
		t.Fatal("vote checkpoints should not be nil")
	}
}

func TestProduceAttestationVoteSourceIsLatestJustified(t *testing.T) {
	fc, _ := buildForkChoiceWithBlocks(t, 5, 2)

	vote := fc.ProduceAttestationVote(3, 0)

	if vote.Source.Slot != fc.LatestJustified.Slot {
		t.Fatalf("vote.Source.Slot = %d, want LatestJustified.Slot = %d",
			vote.Source.Slot, fc.LatestJustified.Slot)
	}
}
