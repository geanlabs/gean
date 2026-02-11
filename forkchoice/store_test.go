package forkchoice

import (
	"testing"

	"github.com/devylongs/gean/consensus"
	"github.com/devylongs/gean/types"
)

// makeTestValidators creates n placeholder validators for testing.
func makeTestValidators(n uint64) []types.Validator {
	validators := make([]types.Validator, n)
	for i := uint64(0); i < n; i++ {
		validators[i] = types.Validator{Index: types.ValidatorIndex(i)}
	}
	return validators
}

// setupTestStore creates a store from genesis for testing.
func setupTestStore(t *testing.T) *Store {
	t.Helper()
	state, block := consensus.GenerateGenesis(1000000000, makeTestValidators(8))
	store, err := NewStore(state, block, consensus.ProcessSlots, consensus.ProcessBlock)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store
}

// buildValidBlock creates a valid block at the given slot using the store's head state.
// Returns the block with a correct state root.
func buildValidBlock(t *testing.T, store *Store, slot types.Slot) *types.Block {
	t.Helper()

	headState := store.States[store.Head]
	headRoot := store.Head

	advanced, err := consensus.ProcessSlots(headState, slot)
	if err != nil {
		t.Fatalf("ProcessSlots to slot %d: %v", slot, err)
	}

	proposer := uint64(slot) % uint64(len(headState.Validators))
	block := &types.Block{
		Slot:          slot,
		ProposerIndex: proposer,
		ParentRoot:    headRoot,
		StateRoot:     types.Root{},
		Body:          types.BlockBody{Attestations: []types.Attestation{}},
	}

	postState, err := consensus.ProcessBlock(advanced, block)
	if err != nil {
		t.Fatalf("ProcessBlock at slot %d: %v", slot, err)
	}

	stateRoot, err := postState.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash post-state: %v", err)
	}
	block.StateRoot = stateRoot

	return block
}

func TestNewStore_Initialization(t *testing.T) {
	state, block := consensus.GenerateGenesis(1000000000, makeTestValidators(8))
	store, err := NewStore(state, block, consensus.ProcessSlots, consensus.ProcessBlock)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Head should be the anchor block root
	anchorRoot, _ := block.HashTreeRoot()
	if store.Head != anchorRoot {
		t.Error("head should be the anchor block root")
	}

	// Should have one block and one state
	if len(store.Blocks) != 1 {
		t.Errorf("blocks count = %d, want 1", len(store.Blocks))
	}
	if len(store.States) != 1 {
		t.Errorf("states count = %d, want 1", len(store.States))
	}

	// Votes slices should be initialized to numValidators length
	if len(store.LatestKnownVotes) != 8 {
		t.Errorf("known votes length = %d, want 8", len(store.LatestKnownVotes))
	}
	if len(store.LatestNewVotes) != 8 {
		t.Errorf("new votes length = %d, want 8", len(store.LatestNewVotes))
	}

	// Config should match
	if store.Config.GenesisTime != 1000000000 {
		t.Errorf("genesis time = %d, want 1000000000", store.Config.GenesisTime)
	}
}

func TestNewStore_AnchorMismatch(t *testing.T) {
	state, block := consensus.GenerateGenesis(1000000000, makeTestValidators(8))
	block.StateRoot = types.Root{0xff} // corrupt the state root

	_, err := NewStore(state, block, consensus.ProcessSlots, consensus.ProcessBlock)
	if err == nil {
		t.Error("expected error for anchor block state root mismatch")
	}
}

func TestStore_ProcessBlock_Valid(t *testing.T) {
	store := setupTestStore(t)

	block := buildValidBlock(t, store, 1)
	if err := store.ProcessBlock(block); err != nil {
		t.Fatalf("ProcessBlock: %v", err)
	}

	blockRoot, _ := block.HashTreeRoot()

	// Block should be stored
	if _, exists := store.Blocks[blockRoot]; !exists {
		t.Error("block should be in store after processing")
	}

	// State should be stored
	if _, exists := store.States[blockRoot]; !exists {
		t.Error("state should be in store after processing")
	}

	// Head should update to the new block
	if store.Head != blockRoot {
		t.Error("head should update to the new block")
	}
}

func TestStore_ProcessBlock_DuplicateSkipped(t *testing.T) {
	store := setupTestStore(t)

	block := buildValidBlock(t, store, 1)
	if err := store.ProcessBlock(block); err != nil {
		t.Fatalf("first ProcessBlock: %v", err)
	}

	blocksBefore := len(store.Blocks)

	// Second time should be a no-op
	if err := store.ProcessBlock(block); err != nil {
		t.Fatalf("second ProcessBlock: %v", err)
	}

	if len(store.Blocks) != blocksBefore {
		t.Error("duplicate block should not add a new entry")
	}
}

func TestStore_ProcessBlock_MissingParent(t *testing.T) {
	store := setupTestStore(t)

	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1,
		ParentRoot:    types.Root{0xff}, // unknown parent
		StateRoot:     types.Root{},
		Body:          types.BlockBody{},
	}

	err := store.ProcessBlock(block)
	if err == nil {
		t.Error("expected error for missing parent")
	}
}

func TestStore_ProcessBlock_InvalidStateRoot(t *testing.T) {
	store := setupTestStore(t)

	block := buildValidBlock(t, store, 1)
	block.StateRoot = types.Root{0xff} // corrupt state root

	err := store.ProcessBlock(block)
	if err == nil {
		t.Error("expected error for invalid state root")
	}
}

func TestStore_HasBlock(t *testing.T) {
	store := setupTestStore(t)

	if !store.HasBlock(store.Head) {
		t.Error("store should have the anchor block")
	}

	if store.HasBlock(types.Root{0xff}) {
		t.Error("store should not have unknown root")
	}
}

func TestStore_GetBlock(t *testing.T) {
	store := setupTestStore(t)

	block, exists := store.GetBlock(store.Head)
	if !exists {
		t.Fatal("anchor block should exist")
	}
	if block.Slot != 0 {
		t.Errorf("anchor block slot = %d, want 0", block.Slot)
	}

	_, exists = store.GetBlock(types.Root{0xff})
	if exists {
		t.Error("unknown root should not exist")
	}
}

func TestStore_MultipleBlocks_HeadUpdates(t *testing.T) {
	store := setupTestStore(t)

	// Add block at slot 1
	block1 := buildValidBlock(t, store, 1)
	if err := store.ProcessBlock(block1); err != nil {
		t.Fatalf("ProcessBlock slot 1: %v", err)
	}

	block1Root, _ := block1.HashTreeRoot()
	if store.Head != block1Root {
		t.Error("head should be block at slot 1")
	}

	// Add block at slot 2
	block2 := buildValidBlock(t, store, 2)
	if err := store.ProcessBlock(block2); err != nil {
		t.Fatalf("ProcessBlock slot 2: %v", err)
	}

	block2Root, _ := block2.HashTreeRoot()
	if store.Head != block2Root {
		t.Error("head should be block at slot 2")
	}

	// Should have 3 blocks total (genesis + 2)
	if len(store.Blocks) != 3 {
		t.Errorf("blocks count = %d, want 3", len(store.Blocks))
	}
}
