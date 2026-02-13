package unit

import (
	"testing"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/storage/memory"
	"github.com/geanlabs/gean/types"
)

func makeGenesisFC(numValidators uint64) (*forkchoice.Store, *types.State) {
	state := statetransition.GenerateGenesis(1000, makeTestValidators(numValidators))

	emptyBody := &types.BlockBody{Attestations: []*types.Attestation{}}
	genesisBlock := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash,
		Body:          emptyBody,
	}
	stateRoot, _ := state.HashTreeRoot()
	genesisBlock.StateRoot = stateRoot

	store := memory.New()
	fc := forkchoice.NewStore(state, genesisBlock, store)
	return fc, state
}

func TestForkChoiceInitFromGenesis(t *testing.T) {
	fc, _ := makeGenesisFC(5)

	if fc.Head == types.ZeroHash {
		t.Error("head should not be zero after genesis")
	}
	if fc.LatestJustified.Root != types.ZeroHash {
		t.Error("genesis justified root should be zero")
	}
}

func TestForkChoiceTickInterval(t *testing.T) {
	fc, _ := makeGenesisFC(5)
	initialTime := fc.Time

	fc.TickInterval(false)
	if fc.Time != initialTime+1 {
		t.Errorf("time = %d, want %d", fc.Time, initialTime+1)
	}
}

func TestForkChoiceAcceptNewAttestations(t *testing.T) {
	fc, _ := makeGenesisFC(5)

	// Add an attestation to new attestations.
	fc.LatestNewAttestations[0] = &types.Attestation{
		ValidatorID: 0,
		Data: &types.AttestationData{
			Slot:   0,
			Head:   &types.Checkpoint{Root: fc.Head, Slot: 0},
			Target: &types.Checkpoint{Root: fc.Head, Slot: 0},
			Source: &types.Checkpoint{Root: fc.Head, Slot: 0},
		},
	}

	fc.AcceptNewAttestations()

	if len(fc.LatestNewAttestations) != 0 {
		t.Error("new attestations should be empty after accept")
	}
	if _, ok := fc.LatestKnownAttestations[0]; !ok {
		t.Error("attestation should be in known attestations after accept")
	}
}

func TestForkChoiceInitPanicsOnAnchorStateRootMismatch(t *testing.T) {
	state := statetransition.GenerateGenesis(1000, makeTestValidators(5))
	emptyBody := &types.BlockBody{Attestations: []*types.Attestation{}}
	genesisBlock := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash, // intentionally wrong
		Body:          emptyBody,
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for anchor block/state root mismatch")
		}
	}()
	_ = forkchoice.NewStore(state, genesisBlock, memory.New())
}

func TestProduceAttestationAcceptsNewAttestationsFirst(t *testing.T) {
	fc, _ := makeGenesisFC(5)
	fc.LatestNewAttestations[3] = &types.Attestation{
		ValidatorID: 3,
		Data: &types.AttestationData{
			Slot:   0,
			Head:   &types.Checkpoint{Root: fc.Head, Slot: 0},
			Target: &types.Checkpoint{Root: fc.Head, Slot: 0},
			Source: &types.Checkpoint{Root: fc.Head, Slot: 0},
		},
	}

	_ = fc.ProduceAttestation(1, 0)

	if len(fc.LatestNewAttestations) != 0 {
		t.Fatalf("expected latest_new_attestations to be drained, got %d entries", len(fc.LatestNewAttestations))
	}
	if _, ok := fc.LatestKnownAttestations[3]; !ok {
		t.Fatal("expected attestation to be moved into latest_known_attestations")
	}
}
