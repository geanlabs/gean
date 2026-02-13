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

	// Add attestations for slot 3 block.
	for i := uint64(0); i < 3; i++ {
		fc.LatestKnownAttestations[i] = &types.Attestation{
			ValidatorID: i,
			Data: &types.AttestationData{
				Slot:   3,
				Head:   &types.Checkpoint{Root: hashes[3], Slot: 3},
				Target: &types.Checkpoint{Root: hashes[3], Slot: 3},
				Source: &types.Checkpoint{Root: hashes[0], Slot: 0},
			},
		}
	}

	block, err := fc.ProduceBlock(4, 4)
	if err != nil {
		t.Fatalf("ProduceBlock: %v", err)
	}

	if len(block.Body.Attestations) == 0 {
		t.Fatal("block should include attestations from known votes")
	}
}

func TestProduceAttestationReturnsValidAttestation(t *testing.T) {
	fc, _ := buildForkChoiceWithBlocks(t, 5, 2)

	att := fc.ProduceAttestation(3, 0)

	if att.ValidatorID != 0 {
		t.Fatalf("att.ValidatorID = %d, want 0", att.ValidatorID)
	}
	if att.Data.Slot != 3 {
		t.Fatalf("att.Data.Slot = %d, want 3", att.Data.Slot)
	}
	if att.Data.Head == nil || att.Data.Target == nil || att.Data.Source == nil {
		t.Fatal("attestation checkpoints should not be nil")
	}
}

func TestProduceAttestationSourceIsLatestJustified(t *testing.T) {
	fc, _ := buildForkChoiceWithBlocks(t, 5, 2)

	att := fc.ProduceAttestation(3, 0)

	if att.Data.Source.Slot != fc.LatestJustified.Slot {
		t.Fatalf("att.Data.Source.Slot = %d, want LatestJustified.Slot = %d",
			att.Data.Source.Slot, fc.LatestJustified.Slot)
	}
}
