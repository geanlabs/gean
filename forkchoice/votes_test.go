package forkchoice

import (
	"errors"
	"testing"

	"github.com/devylongs/gean/consensus"
	"github.com/devylongs/gean/types"
)

// setupStoreWithBlock creates a store with genesis + one valid block at slot 1.
// Returns the store, the block 1 root, and the genesis root.
func setupStoreWithBlock(t *testing.T) (*Store, types.Root, types.Root) {
	t.Helper()
	store := setupTestStore(t)
	genesisRoot := store.Head

	block := buildValidBlock(t, store, 1)
	if err := store.ProcessBlock(block); err != nil {
		t.Fatalf("ProcessBlock: %v", err)
	}

	blockRoot, _ := block.HashTreeRoot()
	return store, blockRoot, genesisRoot
}

func TestValidateAttestation_Valid(t *testing.T) {
	store, blockRoot, genesisRoot := setupStoreWithBlock(t)

	signed := &types.SignedAttestation{
		Message: types.Attestation{
			ValidatorID: 0,
			Data: types.AttestationData{
				Slot:   1,
				Head:   types.Checkpoint{Root: blockRoot, Slot: 1},
				Target: types.Checkpoint{Root: blockRoot, Slot: 1},
				Source: types.Checkpoint{Root: genesisRoot, Slot: 0},
			},
		},
	}

	if err := store.ValidateAttestation(signed); err != nil {
		t.Fatalf("expected valid attestation, got: %v", err)
	}
}

func TestValidateAttestation_GenesisSource(t *testing.T) {
	store, blockRoot, _ := setupStoreWithBlock(t)

	// Genesis source uses zero root and slot 0
	signed := &types.SignedAttestation{
		Message: types.Attestation{
			ValidatorID: 0,
			Data: types.AttestationData{
				Slot:   1,
				Head:   types.Checkpoint{Root: blockRoot, Slot: 1},
				Target: types.Checkpoint{Root: blockRoot, Slot: 1},
				Source: types.Checkpoint{Root: types.Root{}, Slot: 0},
			},
		},
	}

	if err := store.ValidateAttestation(signed); err != nil {
		t.Fatalf("expected valid attestation with genesis source, got: %v", err)
	}
}

func TestValidateAttestation_UnknownTarget(t *testing.T) {
	store, blockRoot, _ := setupStoreWithBlock(t)

	signed := &types.SignedAttestation{
		Message: types.Attestation{
			ValidatorID: 0,
			Data: types.AttestationData{
				Slot:   1,
				Head:   types.Checkpoint{Root: blockRoot, Slot: 1},
				Target: types.Checkpoint{Root: types.Root{0xff}, Slot: 1},
				Source: types.Checkpoint{Root: types.Root{}, Slot: 0},
			},
		},
	}

	err := store.ValidateAttestation(signed)
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	if !errors.Is(err, ErrTargetNotFound) {
		t.Errorf("expected ErrTargetNotFound, got: %v", err)
	}
}

func TestValidateAttestation_SourceAfterTarget(t *testing.T) {
	store, blockRoot, genesisRoot := setupStoreWithBlock(t)

	// Source slot > target slot
	signed := &types.SignedAttestation{
		Message: types.Attestation{
			ValidatorID: 0,
			Data: types.AttestationData{
				Slot:   1,
				Head:   types.Checkpoint{Root: blockRoot, Slot: 1},
				Target: types.Checkpoint{Root: genesisRoot, Slot: 0},
				Source: types.Checkpoint{Root: blockRoot, Slot: 1},
			},
		},
	}

	err := store.ValidateAttestation(signed)
	if err == nil {
		t.Fatal("expected error for source after target")
	}
	if !errors.Is(err, ErrSlotMismatch) {
		t.Errorf("expected ErrSlotMismatch, got: %v", err)
	}
}

func TestValidateAttestation_FutureVote(t *testing.T) {
	store, blockRoot, _ := setupStoreWithBlock(t)

	// Attestation slot far in the future (current slot is ~0 since genesis time is 1B)
	signed := &types.SignedAttestation{
		Message: types.Attestation{
			ValidatorID: 0,
			Data: types.AttestationData{
				Slot:   9999,
				Head:   types.Checkpoint{Root: blockRoot, Slot: 1},
				Target: types.Checkpoint{Root: blockRoot, Slot: 1},
				Source: types.Checkpoint{Root: types.Root{}, Slot: 0},
			},
		},
	}

	err := store.ValidateAttestation(signed)
	if err == nil {
		t.Fatal("expected error for future attestation")
	}
	if !errors.Is(err, ErrFutureVote) {
		t.Errorf("expected ErrFutureVote, got: %v", err)
	}
}

func TestValidateAttestation_UnknownHead(t *testing.T) {
	store, blockRoot, _ := setupStoreWithBlock(t)

	signed := &types.SignedAttestation{
		Message: types.Attestation{
			ValidatorID: 0,
			Data: types.AttestationData{
				Slot:   1,
				Head:   types.Checkpoint{Root: types.Root{0xaa}, Slot: 1},
				Target: types.Checkpoint{Root: blockRoot, Slot: 1},
				Source: types.Checkpoint{Root: types.Root{}, Slot: 0},
			},
		},
	}

	err := store.ValidateAttestation(signed)
	if err == nil {
		t.Fatal("expected error for unknown head")
	}
	if !errors.Is(err, ErrHeadNotFound) {
		t.Errorf("expected ErrHeadNotFound, got: %v", err)
	}
}

func TestValidateAttestation_ValidatorOutOfRange(t *testing.T) {
	store, blockRoot, _ := setupStoreWithBlock(t)

	outOfRangeValidatorID := uint64(len(store.LatestKnownVotes))
	signed := &types.SignedAttestation{
		Message: types.Attestation{
			ValidatorID: outOfRangeValidatorID,
			Data: types.AttestationData{
				Slot:   1,
				Head:   types.Checkpoint{Root: blockRoot, Slot: 1},
				Target: types.Checkpoint{Root: blockRoot, Slot: 1},
				Source: types.Checkpoint{Root: types.Root{}, Slot: 0},
			},
		},
	}

	err := store.ValidateAttestation(signed)
	if err == nil {
		t.Fatal("expected error for out-of-range validator index")
	}
	if !errors.Is(err, ErrValidatorOutOfRange) {
		t.Errorf("expected ErrValidatorOutOfRange, got: %v", err)
	}
}

func TestProcessAttestationLocked_OutOfRangeDoesNotPanic(t *testing.T) {
	store, blockRoot, _ := setupStoreWithBlock(t)

	invalidValidatorID := uint64(len(store.LatestKnownVotes)) + 10
	signed := &types.SignedAttestation{
		Message: types.Attestation{
			ValidatorID: invalidValidatorID,
			Data: types.AttestationData{
				Slot:   1,
				Head:   types.Checkpoint{Root: blockRoot, Slot: 1},
				Target: types.Checkpoint{Root: blockRoot, Slot: 1},
				Source: types.Checkpoint{Root: types.Root{}, Slot: 0},
			},
		},
	}

	// This should safely no-op for both gossip and block paths.
	store.processAttestationLocked(signed, false)
	store.processAttestationLocked(signed, true)
}

func TestProcessAttestation_FromBlock_UpdatesKnown(t *testing.T) {
	state, genesisBlock := consensus.GenerateGenesis(1000000000, makeTestValidators(8))
	store, err := NewStore(state, genesisBlock, consensus.ProcessSlots, consensus.ProcessBlock)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	block := buildValidBlock(t, store, 1)
	blockRoot, _ := block.HashTreeRoot()

	// Add attestations to the block body
	block.Body.Attestations = []types.Attestation{
		{
			ValidatorID: 2,
			Data: types.AttestationData{
				Slot:   1,
				Head:   types.Checkpoint{Root: blockRoot, Slot: 1},
				Target: types.Checkpoint{Root: blockRoot, Slot: 1},
				Source: types.Checkpoint{Root: types.Root{}, Slot: 0},
			},
		},
	}

	// Rebuild the block with attestations to get correct state root
	headState := store.States[store.Head]
	advanced, _ := consensus.ProcessSlots(headState, 1)
	postState, _ := consensus.ProcessBlock(advanced, block)
	stateRoot, _ := postState.HashTreeRoot()
	block.StateRoot = stateRoot

	if err := store.ProcessBlock(block); err != nil {
		t.Fatalf("ProcessBlock: %v", err)
	}

	// Validator 2's known vote should be updated via block attestation processing
	if store.LatestKnownVotes[2].Root.IsZero() {
		t.Error("validator 2 known vote should be set after block with attestation")
	}
}

func TestProcessAttestation_FromGossip_UpdatesNew(t *testing.T) {
	store, blockRoot, _ := setupStoreWithBlock(t)

	// Advance the clock so the attestation isn't "too far in future"
	store.AdvanceTime(1000000000+8, false) // advance past slot 1

	signed := &types.SignedAttestation{
		Message: types.Attestation{
			ValidatorID: 3,
			Data: types.AttestationData{
				Slot:   1,
				Head:   types.Checkpoint{Root: blockRoot, Slot: 1},
				Target: types.Checkpoint{Root: blockRoot, Slot: 1},
				Source: types.Checkpoint{Root: types.Root{}, Slot: 0},
			},
		},
	}

	if err := store.ProcessAttestation(signed); err != nil {
		t.Fatalf("ProcessAttestation: %v", err)
	}

	// Gossip attestation should go to LatestNewVotes
	if store.LatestNewVotes[3].Root.IsZero() {
		t.Error("validator 3 new vote should be set after gossip attestation")
	}
	if store.LatestNewVotes[3].Root != blockRoot {
		t.Error("new vote root should match the target root")
	}
}

func TestAcceptNewVotes_PromotesToKnown(t *testing.T) {
	store, blockRoot, _ := setupStoreWithBlock(t)

	// Manually set a new vote
	store.LatestNewVotes[5] = types.Checkpoint{Root: blockRoot, Slot: 1}

	// Accept new votes
	store.mu.Lock()
	store.acceptNewVotesLocked()
	store.mu.Unlock()

	// New vote should be promoted to known
	if store.LatestKnownVotes[5] != (types.Checkpoint{Root: blockRoot, Slot: 1}) {
		t.Error("new vote should be promoted to known votes")
	}

	// New vote slot should be cleared
	if !store.LatestNewVotes[5].Root.IsZero() {
		t.Error("new vote should be cleared after acceptance")
	}
}
