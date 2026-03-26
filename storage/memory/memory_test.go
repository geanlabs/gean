package memory_test

import (
	"testing"

	"github.com/geanlabs/gean/storage/memory"
	"github.com/geanlabs/gean/types"
)

func TestPutGetBlock(t *testing.T) {
	s := memory.New()
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
	s := memory.New()
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
	s := memory.New()
	_, ok := s.GetBlock([32]byte{0xff})
	if ok {
		t.Fatal("expected missing block to return false")
	}
}

func TestGetMissingStateReturnsFalse(t *testing.T) {
	s := memory.New()
	_, ok := s.GetState([32]byte{0xff})
	if ok {
		t.Fatal("expected missing state to return false")
	}
}

func TestGetAllBlocksCopiesMap(t *testing.T) {
	s := memory.New()
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
	s := memory.New()
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

func TestDeleteBlocksRemovesBlockAndSignedBlock(t *testing.T) {
	s := memory.New()
	root := [32]byte{3}
	s.PutBlock(root, &types.Block{Slot: 2})
	s.PutSignedBlock(root, &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{Block: &types.Block{Slot: 2}},
	})

	s.DeleteBlocks([][32]byte{root})

	if _, ok := s.GetBlock(root); ok {
		t.Fatal("expected block to be deleted")
	}
	if _, ok := s.GetSignedBlock(root); ok {
		t.Fatal("expected signed block to be deleted with block")
	}
}

func TestDeleteStatesRemovesOnlyStates(t *testing.T) {
	s := memory.New()
	root := [32]byte{4}
	s.PutBlock(root, &types.Block{Slot: 3})
	s.PutState(root, &types.State{Slot: 3})

	s.DeleteStates([][32]byte{root})

	if _, ok := s.GetState(root); ok {
		t.Fatal("expected state to be deleted")
	}
	if _, ok := s.GetBlock(root); !ok {
		t.Fatal("expected block to remain after deleting state")
	}
}

func TestMetaRoundTrip(t *testing.T) {
	s := memory.New()

	if err := s.PutMeta("forkchoice/head", []byte{0x01, 0x02}); err != nil {
		t.Fatalf("PutMeta returned error: %v", err)
	}

	value, ok := s.GetMeta("forkchoice/head")
	if !ok {
		t.Fatal("expected metadata to be present")
	}
	if len(value) != 2 || value[0] != 0x01 || value[1] != 0x02 {
		t.Fatalf("unexpected metadata value: %x", value)
	}

	value[0] = 0xFF
	value2, ok := s.GetMeta("forkchoice/head")
	if !ok {
		t.Fatal("expected metadata to remain present")
	}
	if value2[0] != 0x01 {
		t.Fatal("expected GetMeta to return a copy")
	}
}

func TestDeleteMeta(t *testing.T) {
	s := memory.New()

	if err := s.PutMeta("forkchoice/head", []byte{0x01}); err != nil {
		t.Fatalf("PutMeta returned error: %v", err)
	}
	if err := s.DeleteMeta("forkchoice/head"); err != nil {
		t.Fatalf("DeleteMeta returned error: %v", err)
	}
	if _, ok := s.GetMeta("forkchoice/head"); ok {
		t.Fatal("expected metadata to be deleted")
	}
}
