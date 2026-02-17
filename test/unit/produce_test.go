package unit

import (
	"testing"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/types"
)

// testSigner implements forkchoice.Signer for unit tests.
type testSigner struct {
	sig []byte
}

func (s *testSigner) Sign(epoch uint32, message [32]byte) ([]byte, error) {
	if s.sig != nil {
		return s.sig, nil
	}
	out := make([]byte, 3112)
	out[0] = 0xAA // marker
	return out, nil
}

func newTestSigner() forkchoice.Signer {
	return &testSigner{}
}

func TestProduceBlockCreatesValidBlock(t *testing.T) {
	fc, _ := buildForkChoiceWithBlocks(t, 3, 2)

	// Slot 2, proposer = 2 % 3 = 2
	envelope, err := fc.ProduceBlock(2, 2, newTestSigner())
	if err != nil {
		t.Fatalf("ProduceBlock: %v", err)
	}

	block := envelope.Message.Block
	if block.Slot != 2 {
		t.Fatalf("block.Slot = %d, want 2", block.Slot)
	}
	if block.ProposerIndex != 2 {
		t.Fatalf("block.ProposerIndex = %d, want 2", block.ProposerIndex)
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

	// Signed envelope should be stored.
	if _, ok := fc.Storage.GetSignedBlock(blockHash); !ok {
		t.Fatal("produced signed block envelope should be stored")
	}

	// Proposer attestation should be present.
	if envelope.Message.ProposerAttestation == nil {
		t.Fatal("envelope should include proposer attestation")
	}
	if envelope.Message.ProposerAttestation.ValidatorID != 2 {
		t.Fatalf("proposer attestation validator = %d, want 2",
			envelope.Message.ProposerAttestation.ValidatorID)
	}

	// Signature list: body attestations + proposer = at least 1.
	if len(envelope.Signature) == 0 {
		t.Fatal("envelope should have at least one signature slot (proposer)")
	}

	// Proposer signature should be non-zero (signed by testSigner).
	lastIdx := len(envelope.Signature) - 1
	if envelope.Signature[lastIdx][0] != 0xAA {
		t.Fatal("proposer signature should be set by signer")
	}
}

func TestProduceBlockRejectsWrongProposer(t *testing.T) {
	fc, _ := buildForkChoiceWithBlocks(t, 3, 2)

	// Slot 2 proposer is 2, not 0.
	_, err := fc.ProduceBlock(2, 0, newTestSigner())
	if err == nil {
		t.Fatal("expected error for wrong proposer")
	}
}

func TestProduceBlockIncludesAttestations(t *testing.T) {
	fc, hashes := buildForkChoiceWithBlocks(t, 3, 3)

	// Add attestations for slot 2 block.
	for i := uint64(0); i < 2; i++ {
		fc.LatestKnownAttestations[i] = &types.SignedAttestation{
			Message: &types.Attestation{
				ValidatorID: i,
				Data: &types.AttestationData{
					Slot:   2,
					Head:   &types.Checkpoint{Root: hashes[2], Slot: 2},
					Target: &types.Checkpoint{Root: hashes[2], Slot: 2},
					Source: &types.Checkpoint{Root: hashes[0], Slot: 0},
				},
			},
		}
	}

	// Slot 4 proposer = 4 % 3 = 1.
	envelope, err := fc.ProduceBlock(4, 1, newTestSigner())
	if err != nil {
		t.Fatalf("ProduceBlock: %v", err)
	}

	if len(envelope.Message.Block.Body.Attestations) == 0 {
		t.Fatal("block should include attestations from known votes")
	}
}

func TestProduceAttestationReturnsValidAttestation(t *testing.T) {
	fc, _ := buildForkChoiceWithBlocks(t, 3, 2)

	sa, err := fc.ProduceAttestation(2, 0, newTestSigner())
	if err != nil {
		t.Fatalf("ProduceAttestation: %v", err)
	}
	att := sa.Message

	if att.ValidatorID != 0 {
		t.Fatalf("att.ValidatorID = %d, want 0", att.ValidatorID)
	}
	if att.Data.Slot != 2 {
		t.Fatalf("att.Data.Slot = %d, want 2", att.Data.Slot)
	}
	if att.Data.Head == nil || att.Data.Target == nil || att.Data.Source == nil {
		t.Fatal("attestation checkpoints should not be nil")
	}

	// Signature should be non-zero.
	if sa.Signature[0] != 0xAA {
		t.Fatal("attestation signature should be set by signer")
	}
}

func TestProduceAttestationSourceIsLatestJustified(t *testing.T) {
	fc, _ := buildForkChoiceWithBlocks(t, 3, 2)

	sa, err := fc.ProduceAttestation(2, 0, newTestSigner())
	if err != nil {
		t.Fatalf("ProduceAttestation: %v", err)
	}

	if sa.Message.Data.Source.Slot != fc.LatestJustified.Slot {
		t.Fatalf("att.Data.Source.Slot = %d, want LatestJustified.Slot = %d",
			sa.Message.Data.Source.Slot, fc.LatestJustified.Slot)
	}
}
