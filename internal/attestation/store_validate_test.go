package attestation_test

import (
	"testing"

	"github.com/geanlabs/gean/internal/attestation"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func makeValidationStore() *store.ConsensusStore {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	s.SetConfig(&types.ChainConfig{GenesisTime: 1000})
	return s
}

func TestValidateAttestationDataAvailability(t *testing.T) {
	s := makeValidationStore()
	data := &types.AttestationData{
		Slot:   5,
		Source: &types.Checkpoint{Root: [32]byte{1}, Slot: 3},
		Target: &types.Checkpoint{Root: [32]byte{2}, Slot: 4},
		Head:   &types.Checkpoint{Root: [32]byte{3}, Slot: 5},
	}

	err := attestation.ValidateAttestationData(s, data)
	if err == nil {
		t.Fatal("should fail with unknown blocks")
	}
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrUnknownSourceBlock {
		t.Fatalf("expected UnknownSourceBlock, got %v", err)
	}
}

func TestValidateAttestationDataTopology(t *testing.T) {
	s := makeValidationStore()
	s.SetTime(30)

	s.InsertBlockHeader([32]byte{1}, &types.BlockHeader{Slot: 3})
	s.InsertBlockHeader([32]byte{2}, &types.BlockHeader{Slot: 4})
	s.InsertBlockHeader([32]byte{3}, &types.BlockHeader{Slot: 5})

	data := &types.AttestationData{
		Slot:   5,
		Source: &types.Checkpoint{Root: [32]byte{1}, Slot: 3},
		Target: &types.Checkpoint{Root: [32]byte{2}, Slot: 4},
		Head:   &types.Checkpoint{Root: [32]byte{3}, Slot: 5},
	}
	if err := attestation.ValidateAttestationData(s, data); err != nil {
		t.Fatalf("should pass: %v", err)
	}

	bad := *data
	bad.Source = &types.Checkpoint{Root: [32]byte{3}, Slot: 5}
	bad.Target = &types.Checkpoint{Root: [32]byte{1}, Slot: 3}
	err := attestation.ValidateAttestationData(s, &bad)
	if err == nil {
		t.Fatal("should fail: source exceeds target")
	}
}
