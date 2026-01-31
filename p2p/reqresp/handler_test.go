package reqresp

import (
	"testing"

	"github.com/devylongs/gean/chain"
	"github.com/devylongs/gean/forkchoice"
	"github.com/devylongs/gean/types"
)

func setupTestStore(t *testing.T) *forkchoice.Store {
	genesisState := chain.GenerateGenesis(1000, 4)

	genesisBlock := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.Root{},
		StateRoot:     types.Root{},
		Body:          types.BlockBody{Attestations: []types.SignedVote{}},
	}

	stateRoot, _ := genesisState.HashTreeRoot()
	genesisBlock.StateRoot = stateRoot

	store, err := forkchoice.NewStore(genesisState, genesisBlock)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	return store
}

func TestGetStatus(t *testing.T) {
	store := setupTestStore(t)
	handler := NewHandler(store)

	status := handler.GetStatus()

	if status == nil {
		t.Fatal("GetStatus returned nil")
	}

	// Genesis state should have zero finalized slot
	if status.Finalized.Slot != 0 {
		t.Errorf("Finalized.Slot = %d, want 0", status.Finalized.Slot)
	}

	// Head should match store head
	if status.Head.Root != store.Head {
		t.Error("Head.Root does not match store head")
	}
}

func TestHandleBlocksByRoot(t *testing.T) {
	store := setupTestStore(t)
	handler := NewHandler(store)

	// Request the genesis block
	genesisRoot := store.Head

	request := &types.BlocksByRootRequest{
		Roots: []types.Root{genesisRoot},
	}

	blocks := handler.HandleBlocksByRoot(request)

	if len(blocks) != 1 {
		t.Errorf("Expected 1 block, got %d", len(blocks))
	}

	if blocks[0].Message.Slot != 0 {
		t.Errorf("Block slot = %d, want 0", blocks[0].Message.Slot)
	}
}

func TestHandleBlocksByRootUnknown(t *testing.T) {
	store := setupTestStore(t)
	handler := NewHandler(store)

	// Request an unknown block
	unknownRoot := types.Root{1, 2, 3}

	request := &types.BlocksByRootRequest{
		Roots: []types.Root{unknownRoot},
	}

	blocks := handler.HandleBlocksByRoot(request)

	if len(blocks) != 0 {
		t.Errorf("Expected 0 blocks for unknown root, got %d", len(blocks))
	}
}

func TestValidatePeerStatus(t *testing.T) {
	store := setupTestStore(t)
	handler := NewHandler(store)

	// Valid status (genesis)
	validStatus := &types.Status{
		Finalized: types.Checkpoint{Root: types.Root{}, Slot: 0},
		Head:      types.Checkpoint{Root: store.Head, Slot: 0},
	}

	err := handler.ValidatePeerStatus(validStatus)
	if err != nil {
		t.Errorf("ValidatePeerStatus failed for valid status: %v", err)
	}
}
