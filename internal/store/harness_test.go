package store_test

import (
	"errors"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func makeTestStore() *store.ConsensusStore {
	backend := storage.NewInMemoryBackend()
	s := store.NewConsensusStore(backend)
	s.SetConfig(&types.ChainConfig{GenesisTime: 1000})
	return s
}

func makeCheckpoint(rootByte byte, slot uint64) *types.Checkpoint {
	var root [32]byte
	root[0] = rootByte
	return &types.Checkpoint{Root: root, Slot: slot}
}

func makeHeader(slot, proposer uint64, parentRootByte byte) *types.BlockHeader {
	var parent [32]byte
	parent[0] = parentRootByte
	return &types.BlockHeader{
		Slot:          slot,
		ProposerIndex: proposer,
		ParentRoot:    parent,
	}
}

type failingWriteBackend struct {
	*storage.InMemoryBackend
}

func (b failingWriteBackend) BeginWrite() (storage.WriteBatch, error) {
	return nil, errors.New("begin write failed")
}
