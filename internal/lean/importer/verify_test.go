package importer

import (
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/core/storage"
	"github.com/geanlabs/gean/internal/lean/store"
	"github.com/geanlabs/gean/internal/lean/types"
)

func TestVerifyBlockSignaturesRejectsMissingProof(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	err := verifyBlockSignatures(s, &types.SignedBlock{
		Block: processorBlock(),
		Proof: &types.MultiMessageAggregate{},
	}, &types.State{Validators: []*types.Validator{{}}})
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrAttestationSignatureMismatch {
		t.Fatalf("expected missing proof error, got %v", err)
	}
}

func TestVerifyBlockSignaturesRejectsEmptyParticipants(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	err := verifyBlockSignatures(s, &types.SignedBlock{
		Block: processorBlock(processorAttestation()),
		Proof: &types.MultiMessageAggregate{Proof: []byte{1}},
	}, &types.State{Validators: []*types.Validator{{}}})
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrParticipantsMismatch {
		t.Fatalf("expected participant mismatch error, got %v", err)
	}
}

// XMSS signs 32-bit slots, so a slot past that, on an attestation or on the
// block itself, fails the block proof. OnBlock refuses such a block slot as too
// far in the future first; this checks the proof would too.
func TestVerifyBlockSignaturesRejectsSlotOverflow(t *testing.T) {
	att := processorAttestation()
	att.Data.Slot = ^uint64(0)
	types.BitlistSet(att.AggregationBits, 0)
	block := processorBlock()
	block.Slot = ^uint64(0)
	for name, b := range map[string]*types.Block{"attestation": processorBlock(att), "block": block} {
		t.Run(name, func(t *testing.T) {
			s := store.NewConsensusStore(storage.NewInMemoryBackend())
			err := verifyBlockSignatures(s, &types.SignedBlock{
				Block: b,
				Proof: &types.MultiMessageAggregate{Proof: []byte{1}},
			}, &types.State{Validators: []*types.Validator{{}}})
			se, ok := err.(*store.StoreError)
			if !ok || se.Kind != store.ErrAggregateVerificationFailed || !strings.Contains(err.Error(), "overflows uint32") {
				t.Fatalf("expected a slot overflow rejecting the block proof, got %v", err)
			}
		})
	}
}

func TestVerifyBlockSignaturesRejectsNilProposerValidator(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	err := verifyBlockSignatures(s, &types.SignedBlock{
		Block: processorBlock(),
		Proof: &types.MultiMessageAggregate{Proof: []byte{1}},
	}, &types.State{Validators: []*types.Validator{nil}})
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrInvalidValidatorIndex {
		t.Fatalf("expected invalid validator index error, got %v", err)
	}
}
