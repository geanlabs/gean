package memory

import (
	"testing"

	"github.com/geanlabs/gean/types"
)

func TestPutGetBlock(t *testing.T) {
	s := New()
	root := [32]byte{1}
	block := &types.Block{Slot: 5}

	s.PutBlock(root, block)

	got, ok := s.GetBlock(root)
	if !ok {
		t.Fatal("expected block to be found")
	}
	if got.Slot != 5 {
		t.Fatalf("block slot = %d, want 5", got.Slot)
	}
}

func TestPutGetState(t *testing.T) {
	s := New()
	root := [32]byte{2}
	state := &types.State{Slot: 10}

	s.PutState(root, state)

	got, ok := s.GetState(root)
	if !ok {
		t.Fatal("expected state to be found")
	}
	if got.Slot != 10 {
		t.Fatalf("state slot = %d, want 10", got.Slot)
	}
}

func TestGetMissingBlockReturnsFalse(t *testing.T) {
	s := New()
	_, ok := s.GetBlock([32]byte{0xff})
	if ok {
		t.Fatal("expected missing block to return false")
	}
}

func TestGetMissingStateReturnsFalse(t *testing.T) {
	s := New()
	_, ok := s.GetState([32]byte{0xff})
	if ok {
		t.Fatal("expected missing state to return false")
	}
}

func TestGetAllBlocksCopiesMap(t *testing.T) {
	s := New()
	root := [32]byte{1}
	block := &types.Block{Slot: 1}
	s.PutBlock(root, block)

	all := s.GetAllBlocks()
	// Mutating the returned map should not affect the store.
	delete(all, root)

	_, ok := s.GetBlock(root)
	if !ok {
		t.Fatal("deleting from GetAllBlocks result should not affect store")
	}
}

func TestGetAllStatesCopiesMap(t *testing.T) {
	s := New()
	root := [32]byte{1}
	state := &types.State{Slot: 1}
	s.PutState(root, state)

	all := s.GetAllStates()
	delete(all, root)

	_, ok := s.GetState(root)
	if !ok {
		t.Fatal("deleting from GetAllStates result should not affect store")
	}
}
