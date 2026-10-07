package store_test

import (
	"errors"
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// putFailingBackend fails every PutBatch after the first, so a write that is
// split across several batches would leave the first part stored.
type putFailingBackend struct {
	*storage.InMemoryBackend
}

func (b putFailingBackend) BeginWrite() (storage.WriteBatch, error) {
	wb, err := b.InMemoryBackend.BeginWrite()
	if err != nil {
		return nil, err
	}
	return &putFailingBatch{WriteBatch: wb}, nil
}

type putFailingBatch struct {
	storage.WriteBatch
	calls int
}

func (b *putFailingBatch) PutBatch(table storage.Table, entries []storage.KV) error {
	b.calls++
	if b.calls > 1 {
		return errors.New("put failed")
	}
	return b.WriteBatch.PutBatch(table, entries)
}

func importedBlock(t *testing.T) ([32]byte, *types.SignedBlock, *types.State) {
	t.Helper()
	block := &types.Block{Slot: 1, Body: &types.BlockBody{}}
	root, err := block.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash block: %v", err)
	}
	postState := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     1,
		LatestBlockHeader:        &types.BlockHeader{Slot: 1},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
	return root, &types.SignedBlock{Block: block, Proof: &types.MultiMessageAggregate{}}, postState
}

func TestPutImportedBlockIsAtomic(t *testing.T) {
	s := store.NewConsensusStore(putFailingBackend{InMemoryBackend: storage.NewInMemoryBackend()})
	root, signed, postState := importedBlock(t)

	if err := s.PutImportedBlock(root, signed, postState); err == nil {
		t.Fatal("expected batch write error")
	}
	if s.GetBlockHeader(root) != nil || s.HasState(root) || s.GetSignedBlock(root) != nil {
		t.Fatal("block was partially persisted")
	}
	if got := s.MaxStoredBlockSlot(); got != 0 {
		t.Fatalf("max stored block slot=%d after failed write, want 0", got)
	}
}

func TestPutImportedBlockAdvancesOnlyJustified(t *testing.T) {
	s := makeTestStore()
	root, signed, postState := importedBlock(t)
	postState.LatestJustified = &types.Checkpoint{Slot: 1, Root: root}
	postState.LatestFinalized = &types.Checkpoint{Slot: 1, Root: root}
	finalizedBefore := s.LatestFinalized()

	if err := s.PutImportedBlock(root, signed, postState); err != nil {
		t.Fatalf("persist block: %v", err)
	}
	if got := s.LatestJustified(); got == nil || got.Slot != 1 || got.Root != root {
		t.Fatalf("latest justified=%v, want slot 1 root 0x%x", got, root)
	}
	// Finalization is derived from the canonical head during head selection,
	// never persisted by the import path, so this write must leave it untouched.
	if got := s.LatestFinalized(); *got != *finalizedBefore {
		t.Fatalf("latest finalized=%v, want unchanged %v", got, finalizedBefore)
	}
	if s.GetBlockHeader(root) == nil || !s.HasState(root) || s.GetSignedBlock(root) == nil {
		t.Fatal("block was not persisted")
	}
}
