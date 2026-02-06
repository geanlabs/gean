package reqresp

import (
	"testing"

	"github.com/devylongs/gean/types"
)

// mockBlockReader satisfies BlockReader for testing without importing forkchoice.
type mockBlockReader struct {
	head      types.Root
	blocks    map[types.Root]*types.Block
	finalized types.Checkpoint
}

func (m *mockBlockReader) GetHead() types.Root                            { return m.head }
func (m *mockBlockReader) GetBlock(root types.Root) (*types.Block, bool)  { b, ok := m.blocks[root]; return b, ok }
func (m *mockBlockReader) GetLatestFinalized() types.Checkpoint           { return m.finalized }

func newMockStore() (*mockBlockReader, types.Root) {
	genesisBlock := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.Root{},
		StateRoot:     types.Root{},
		Body:          types.BlockBody{},
	}

	root, _ := genesisBlock.HashTreeRoot()

	mock := &mockBlockReader{
		head:      root,
		blocks:    map[types.Root]*types.Block{root: genesisBlock},
		finalized: types.Checkpoint{Root: types.Root{}, Slot: 0},
	}

	return mock, root
}

func TestGetStatus(t *testing.T) {
	mock, genesisRoot := newMockStore()
	handler := NewHandler(mock)

	status := handler.GetStatus()
	if status == nil {
		t.Fatal("GetStatus returned nil")
	}

	if status.Finalized.Slot != 0 {
		t.Errorf("Finalized.Slot = %d, want 0", status.Finalized.Slot)
	}

	if status.Head.Root != genesisRoot {
		t.Error("Head.Root does not match genesis root")
	}
}

func TestHandleBlocksByRoot(t *testing.T) {
	mock, genesisRoot := newMockStore()
	handler := NewHandler(mock)

	request := &BlocksByRootRequest{
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
	mock, _ := newMockStore()
	handler := NewHandler(mock)

	request := &BlocksByRootRequest{
		Roots: []types.Root{{1, 2, 3}},
	}

	blocks := handler.HandleBlocksByRoot(request)

	if len(blocks) != 0 {
		t.Errorf("Expected 0 blocks for unknown root, got %d", len(blocks))
	}
}

func TestValidatePeerStatus(t *testing.T) {
	mock, genesisRoot := newMockStore()
	handler := NewHandler(mock)

	validStatus := &Status{
		Finalized: types.Checkpoint{Root: types.Root{}, Slot: 0},
		Head:      types.Checkpoint{Root: genesisRoot, Slot: 0},
	}

	err := handler.ValidatePeerStatus(validStatus)
	if err != nil {
		t.Errorf("ValidatePeerStatus failed for valid status: %v", err)
	}
}
