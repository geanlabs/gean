package store_test

import (
	"errors"
	"testing"

	"github.com/geanlabs/gean/internal/core/storage"
	"github.com/geanlabs/gean/internal/lean/store"
	"github.com/geanlabs/gean/internal/lean/types"
)

func anchorFixture() ([32]byte, *types.BlockHeader, *types.State) {
	header := makeHeader(4, 1, 0x0a)
	state := &types.State{Config: &types.ChainConfig{GenesisTime: 1000}, Slot: 4, LatestBlockHeader: header}
	return [32]byte{0xa4}, header, state
}

func TestPutAnchorWritesEveryRow(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	root, header, state := anchorFixture()
	if err := s.PutAnchor(root, header, state); err != nil {
		t.Fatal(err)
	}

	anchor := &types.Checkpoint{Root: root, Slot: 4}
	if s.Head() != root || s.SafeTarget() != root {
		t.Fatalf("head=%x safe target=%x, want the anchor %x", s.Head(), s.SafeTarget(), root)
	}
	if got := s.LatestJustified(); *got != *anchor {
		t.Fatalf("justified=%+v, want %+v", got, anchor)
	}
	if got := s.LatestFinalized(); *got != *anchor {
		t.Fatalf("finalized=%+v, want %+v", got, anchor)
	}
	if got := s.Config(); got == nil || got.GenesisTime != 1000 {
		t.Fatalf("config=%+v, want genesis time 1000", got)
	}
	if got := s.GetBlockHeader(root); got == nil || *got != *header {
		t.Fatalf("header=%+v, want %+v", got, header)
	}
	if !s.HasState(root) {
		t.Fatal("anchor state missing")
	}
	entries, err := s.LiveChainRange(0, 0)
	if err != nil || len(entries) != 1 || entries[0].Root != root || entries[0].Slot != 4 {
		t.Fatalf("live chain=%+v err=%v, want the anchor at slot 4", entries, err)
	}
}

func TestPutAnchorFailedCommitLeavesNothing(t *testing.T) {
	s := store.NewConsensusStore(failingCommitBackend{storage.NewInMemoryBackend()})
	root, header, state := anchorFixture()
	if err := s.PutAnchor(root, header, state); err == nil {
		t.Fatal("expected the commit to fail")
	}
	if s.Head() != types.ZeroRoot || s.GetBlockHeader(root) != nil || s.HasState(root) {
		t.Fatal("a failed anchor write left rows behind")
	}
}

type failingCommitBackend struct {
	*storage.InMemoryBackend
}

func (b failingCommitBackend) BeginWrite() (storage.WriteBatch, error) {
	wb, err := b.InMemoryBackend.BeginWrite()
	return failingCommitBatch{wb}, err
}

type failingCommitBatch struct {
	storage.WriteBatch
}

func (failingCommitBatch) Commit() error { return errors.New("commit failed") }
