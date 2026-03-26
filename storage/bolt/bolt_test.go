package bolt_test

import (
	"path/filepath"
	"testing"

	boltstore "github.com/geanlabs/gean/storage/bolt"
	"github.com/geanlabs/gean/types"
)

func newTestStore(t *testing.T) *boltstore.Store {
	t.Helper()
	s, err := boltstore.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open bolt store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPutGetBlock(t *testing.T) {
	s := newTestStore(t)
	root := [32]byte{1}
	block := &types.Block{Slot: 5, Body: &types.BlockBody{}}

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
	s := newTestStore(t)
	root := [32]byte{2}
	state := &types.State{
		Slot:                     10,
		Config:                   &types.Config{GenesisTime: 1000},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		LatestBlockHeader:        &types.BlockHeader{},
		JustifiedSlots:           []byte{0x01},
		JustificationsValidators: []byte{0x01},
	}

	s.PutState(root, state)

	got, ok := s.GetState(root)
	if !ok {
		t.Fatal("expected state to be found")
	}
	if got.Slot != 10 {
		t.Fatalf("state slot = %d, want 10", got.Slot)
	}
}

func TestPutGetSignedBlock(t *testing.T) {
	s := newTestStore(t)
	root := [32]byte{3}
	sb := &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{
			Block: &types.Block{Slot: 7, Body: &types.BlockBody{}},
		},
	}

	s.PutSignedBlock(root, sb)

	got, ok := s.GetSignedBlock(root)
	if !ok {
		t.Fatal("expected signed block to be found")
	}
	if got.Message.Block.Slot != 7 {
		t.Fatalf("signed block slot = %d, want 7", got.Message.Block.Slot)
	}
}

func TestGetMissingBlockReturnsFalse(t *testing.T) {
	s := newTestStore(t)
	_, ok := s.GetBlock([32]byte{0xff})
	if ok {
		t.Fatal("expected missing block to return false")
	}
}

func TestGetMissingStateReturnsFalse(t *testing.T) {
	s := newTestStore(t)
	_, ok := s.GetState([32]byte{0xff})
	if ok {
		t.Fatal("expected missing state to return false")
	}
}

func TestGetMissingSignedBlockReturnsFalse(t *testing.T) {
	s := newTestStore(t)
	_, ok := s.GetSignedBlock([32]byte{0xff})
	if ok {
		t.Fatal("expected missing signed block to return false")
	}
}

func TestGetAllBlocksCopiesMap(t *testing.T) {
	s := newTestStore(t)
	root := [32]byte{1}
	block := &types.Block{Slot: 1, Body: &types.BlockBody{}}
	s.PutBlock(root, block)

	all := s.GetAllBlocks()
	delete(all, root)

	_, ok := s.GetBlock(root)
	if !ok {
		t.Fatal("deleting from GetAllBlocks result should not affect store")
	}
}

func TestGetAllStatesCopiesMap(t *testing.T) {
	s := newTestStore(t)
	root := [32]byte{1}
	state := &types.State{
		Slot:                     1,
		Config:                   &types.Config{GenesisTime: 1000},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		LatestBlockHeader:        &types.BlockHeader{},
		JustifiedSlots:           []byte{0x01},
		JustificationsValidators: []byte{0x01},
	}
	s.PutState(root, state)

	all := s.GetAllStates()
	delete(all, root)

	_, ok := s.GetState(root)
	if !ok {
		t.Fatal("deleting from GetAllStates result should not affect store")
	}
}

func TestClose(t *testing.T) {
	s, err := boltstore.New(filepath.Join(t.TempDir(), "close.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestDeleteBlocksRemovesBlockAndSignedBlock(t *testing.T) {
	s := newTestStore(t)
	root := [32]byte{4}
	s.PutBlock(root, &types.Block{Slot: 9, Body: &types.BlockBody{}})
	s.PutSignedBlock(root, &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{Block: &types.Block{Slot: 9, Body: &types.BlockBody{}}},
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
	s := newTestStore(t)
	root := [32]byte{5}
	s.PutBlock(root, &types.Block{Slot: 10, Body: &types.BlockBody{}})
	s.PutState(root, &types.State{
		Slot:                     10,
		Config:                   &types.Config{GenesisTime: 1000},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		LatestBlockHeader:        &types.BlockHeader{},
		JustifiedSlots:           []byte{0x01},
		JustificationsValidators: []byte{0x01},
	})

	s.DeleteStates([][32]byte{root})

	if _, ok := s.GetState(root); ok {
		t.Fatal("expected state to be deleted")
	}
	if _, ok := s.GetBlock(root); !ok {
		t.Fatal("expected block to remain after deleting state")
	}
}
