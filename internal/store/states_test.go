package store_test

import (
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func TestStateStorage(t *testing.T) {
	s := makeTestStore()
	var root [32]byte
	root[0] = 0x01

	state := makeState(10)
	s.InsertState(root, state)
	if !s.HasState(root) {
		t.Fatal("state should exist")
	}
	got := s.GetState(root)
	if got == nil {
		t.Fatal("state not found")
	}
	if got.Slot != 10 {
		t.Fatalf("state slot mismatch: expected 10, got %d", got.Slot)
	}
}

func makeState(slot uint64) *types.State {
	return &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     slot,
		LatestBlockHeader:        &types.BlockHeader{},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
}

func TestInsertStateIgnoresNil(t *testing.T) {
	s := makeTestStore()
	var root [32]byte
	root[0] = 1

	s.InsertState(root, nil)
	if s.HasState(root) {
		t.Fatal("nil state should not be stored")
	}
}

func TestPutStateReturnsInputAndWriteErrors(t *testing.T) {
	s := makeTestStore()
	var root [32]byte
	if err := s.PutState(root, nil); err == nil {
		t.Fatal("expected nil state error")
	}

	s = store.NewConsensusStore(failingWriteBackend{InMemoryBackend: storage.NewInMemoryBackend()})
	if err := s.PutState(root, makeState(1)); err == nil {
		t.Fatal("expected write error")
	}
}
