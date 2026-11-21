package store_test

import (
	"testing"

	"github.com/geanlabs/gean/internal/core/storage"
	"github.com/geanlabs/gean/internal/lean/store"
	"github.com/geanlabs/gean/internal/lean/types"
)

func TestBlockHeaderStorage(t *testing.T) {
	s := makeTestStore()
	var root [32]byte
	root[0] = 0x01
	h := makeHeader(5, 2, 0x00)

	s.InsertBlockHeader(root, h)
	got := s.GetBlockHeader(root)
	if got == nil {
		t.Fatal("header not found")
	}
	if got.Slot != 5 || got.ProposerIndex != 2 {
		t.Fatalf("header mismatch: slot=%d proposer=%d", got.Slot, got.ProposerIndex)
	}
}

func TestBlockWritesIgnoreMalformedInput(t *testing.T) {
	s := makeTestStore()
	var root [32]byte
	root[0] = 1

	s.InsertBlockHeader(root, nil)
	if got := s.GetBlockHeader(root); got != nil {
		t.Fatal("nil header should not be stored")
	}

}

func TestPutBlockHeaderReturnsInputAndWriteErrors(t *testing.T) {
	s := makeTestStore()
	var root [32]byte
	if err := s.PutBlockHeader(root, nil); err == nil {
		t.Fatal("expected nil header error")
	}

	s = store.NewConsensusStore(failingWriteBackend{InMemoryBackend: storage.NewInMemoryBackend()})
	if err := s.PutBlockHeader(root, makeHeader(1, 0, 0)); err == nil {
		t.Fatal("expected write error")
	}
}

func TestWriteBlockDataReturnsErrors(t *testing.T) {
	var root [32]byte
	if err := store.WriteBlockData(nil, root, makeSignedBlock()); err == nil {
		t.Fatal("expected nil store error")
	}

	s := makeTestStore()
	if err := store.WriteBlockData(s, root, nil); err == nil {
		t.Fatal("expected nil signed block error")
	}
	if err := store.WriteBlockData(s, root, &types.SignedBlock{}); err == nil {
		t.Fatal("expected missing block error")
	}
	if err := store.WriteBlockData(s, root, &types.SignedBlock{Block: &types.Block{}, Proof: &types.MultiMessageAggregate{}}); err == nil {
		t.Fatal("expected missing block body error")
	}
	if got := s.GetSignedBlock(root); got != nil {
		t.Fatal("malformed signed block should not be stored")
	}

	s = store.NewConsensusStore(failingWriteBackend{InMemoryBackend: storage.NewInMemoryBackend()})
	if err := store.WriteBlockData(s, root, makeSignedBlock()); err == nil {
		t.Fatal("expected write error")
	}
}

func TestStorePendingBlockReturnsErrors(t *testing.T) {
	var root [32]byte
	s := makeTestStore()
	if err := s.StorePendingBlock(root, nil); err == nil {
		t.Fatal("expected nil signed block error")
	}
	if err := s.StorePendingBlock(root, &types.SignedBlock{}); err == nil {
		t.Fatal("expected missing block error")
	}
	if got := s.GetSignedBlock(root); got != nil {
		t.Fatal("malformed signed block should not be stored")
	}

	s = store.NewConsensusStore(failingWriteBackend{InMemoryBackend: storage.NewInMemoryBackend()})
	if err := s.StorePendingBlock(root, makeSignedBlock()); err == nil {
		t.Fatal("expected write error")
	}
}

func makeSignedBlock() *types.SignedBlock {
	return &types.SignedBlock{
		Block: &types.Block{
			Slot: 1,
			Body: &types.BlockBody{},
		},
		Proof: &types.MultiMessageAggregate{},
	}
}
