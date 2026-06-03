package blockprocessor

import (
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func processorProof(att *types.AggregatedAttestation) *types.AggregatedSignatureProof {
	return &types.AggregatedSignatureProof{
		Participants: att.AggregationBits,
		ProofData:    []byte{0x01},
	}
}

func TestBuildVerifyJobsRejectsMissingProof(t *testing.T) {
	s := &store.ConsensusStore{}
	block := processorBlock(processorAttestation())
	sigs := &types.BlockSignatures{}

	jobs, err := buildVerifyJobs(s, block, sigs)
	if jobs != nil {
		t.Fatalf("jobs=%v, want nil", jobs)
	}
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrAttestationSignatureMismatch {
		t.Fatalf("expected signature mismatch error, got %v", err)
	}
}

func TestBuildVerifyJobsRejectsNilProof(t *testing.T) {
	s := &store.ConsensusStore{}
	block := processorBlock(processorAttestation())
	sigs := &types.BlockSignatures{AttestationSignatures: []*types.AggregatedSignatureProof{nil}}

	jobs, err := buildVerifyJobs(s, block, sigs)
	if jobs != nil {
		t.Fatalf("jobs=%v, want nil", jobs)
	}
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrAttestationSignatureMismatch {
		t.Fatalf("expected signature mismatch error, got %v", err)
	}
}

func TestBuildVerifyJobsRejectsEmptyProofData(t *testing.T) {
	s := &store.ConsensusStore{}
	att := processorAttestation()
	types.BitlistSet(att.AggregationBits, 0)
	block := processorBlock(att)
	sigs := &types.BlockSignatures{
		AttestationSignatures: []*types.AggregatedSignatureProof{{
			Participants: att.AggregationBits,
		}},
	}

	jobs, err := buildVerifyJobs(s, block, sigs)
	if jobs != nil {
		t.Fatalf("jobs=%v, want nil", jobs)
	}
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrAttestationSignatureMismatch {
		t.Fatalf("expected signature mismatch error, got %v", err)
	}
}

func TestBuildVerifyJobsRejectsParticipantMismatch(t *testing.T) {
	s := &store.ConsensusStore{}
	att := processorAttestation()
	types.BitlistSet(att.AggregationBits, 0)
	participants := types.NewBitlistSSZ(2)
	types.BitlistSet(participants, 1)

	jobs, err := buildVerifyJobs(s, processorBlock(att), &types.BlockSignatures{
		AttestationSignatures: []*types.AggregatedSignatureProof{{
			Participants: participants,
			ProofData:    []byte{0x01},
		}},
	})
	if jobs != nil {
		t.Fatalf("jobs=%v, want nil", jobs)
	}
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrParticipantsMismatch {
		t.Fatalf("expected participants mismatch error, got %v", err)
	}
}

func TestBuildVerifyJobsRejectsMissingTargetState(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	att := processorAttestation()
	types.BitlistSet(att.AggregationBits, 0)

	jobs, err := buildVerifyJobs(s, processorBlock(att), &types.BlockSignatures{
		AttestationSignatures: []*types.AggregatedSignatureProof{processorProof(att)},
	})
	if jobs != nil {
		t.Fatalf("jobs=%v, want nil", jobs)
	}
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrMissingTargetState {
		t.Fatalf("expected missing target state error, got %v", err)
	}
}

func TestBuildVerifyJobsRejectsAttestationSlotOverflow(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	att := processorAttestation()
	att.Data.Slot = ^uint64(0)
	types.BitlistSet(att.AggregationBits, 0)

	jobs, err := buildVerifyJobs(s, processorBlock(att), &types.BlockSignatures{
		AttestationSignatures: []*types.AggregatedSignatureProof{processorProof(att)},
	})
	if jobs != nil {
		t.Fatalf("jobs=%v, want nil", jobs)
	}
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrSignatureDecodingFailed {
		t.Fatalf("expected signature decoding error, got %v", err)
	}
}

func TestBuildVerifyJobsRejectsExtraProof(t *testing.T) {
	s := &store.ConsensusStore{}
	jobs, err := buildVerifyJobs(s, processorBlock(), &types.BlockSignatures{
		AttestationSignatures: []*types.AggregatedSignatureProof{{ProofData: []byte{0x01}}},
	})
	if jobs != nil {
		t.Fatalf("jobs=%v, want nil", jobs)
	}
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrAttestationSignatureMismatch {
		t.Fatalf("expected signature mismatch error, got %v", err)
	}
}

func TestBuildVerifyJobsRejectsMalformedBlock(t *testing.T) {
	s := &store.ConsensusStore{}
	jobs, err := buildVerifyJobs(s, &types.Block{}, &types.BlockSignatures{})
	if jobs != nil {
		t.Fatalf("jobs=%v, want nil", jobs)
	}
	if err == nil {
		t.Fatal("expected malformed block error")
	}
}

func TestRunVerifyJobsAllowsEmptyBatch(t *testing.T) {
	if err := runVerifyJobs(nil); err != nil {
		t.Fatalf("run empty jobs: %v", err)
	}
}

func TestVerifyBlockSignaturesRejectsBlockSlotOverflow(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	state := &types.State{Validators: []*types.Validator{{}}}
	block := processorBlock()
	block.Slot = ^uint64(0)
	signedBlock := &types.SignedBlock{
		Block:     block,
		Signature: &types.BlockSignatures{},
	}

	err := verifyBlockSignatures(s, signedBlock, state)
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrProposerSignatureDecodingFailed {
		t.Fatalf("expected proposer signature decoding error, got %v", err)
	}
}

func TestVerifyBlockSignaturesRejectsNilProposerValidator(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	state := &types.State{Validators: []*types.Validator{nil}}
	signedBlock := &types.SignedBlock{
		Block:     processorBlock(),
		Signature: &types.BlockSignatures{},
	}

	err := verifyBlockSignatures(s, signedBlock, state)
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrInvalidValidatorIndex {
		t.Fatalf("expected invalid validator index error, got %v", err)
	}
}

func TestParticipantPubkeysRejectsNilTargetState(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	proof := &types.AggregatedSignatureProof{Participants: types.BitlistFromIndices([]uint64{0})}

	pubkeys, err := participantPubkeys(s, nil, proof)
	if pubkeys != nil {
		t.Fatalf("pubkeys=%v, want nil", pubkeys)
	}
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrMissingTargetState {
		t.Fatalf("expected missing target state error, got %v", err)
	}
}

func TestParticipantPubkeysRejectsNilPubkeyCache(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	s.PubKeyCache = nil
	state := &types.State{Validators: []*types.Validator{{}}}
	proof := &types.AggregatedSignatureProof{Participants: types.BitlistFromIndices([]uint64{0})}

	pubkeys, err := participantPubkeys(s, state, proof)
	if pubkeys != nil {
		t.Fatalf("pubkeys=%v, want nil", pubkeys)
	}
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrPubkeyDecodingFailed {
		t.Fatalf("expected pubkey decode error, got %v", err)
	}
}

func TestParticipantPubkeysRejectsNilValidator(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	state := &types.State{Validators: []*types.Validator{nil}}
	proof := &types.AggregatedSignatureProof{Participants: types.BitlistFromIndices([]uint64{0})}

	pubkeys, err := participantPubkeys(s, state, proof)
	if pubkeys != nil {
		t.Fatalf("pubkeys=%v, want nil", pubkeys)
	}
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrInvalidValidatorIndex {
		t.Fatalf("expected invalid validator index error, got %v", err)
	}
}
