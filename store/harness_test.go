package store_test

import (
	"errors"

	"github.com/geanlabs/gean/db"
	"github.com/geanlabs/gean/store"
	"github.com/geanlabs/gean/types"
)

func makeTestStore() *store.ConsensusStore {
	backend := db.NewInMemoryBackend()
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
	*db.InMemoryBackend
}

func (b failingWriteBackend) BeginWrite() (db.WriteBatch, error) {
	return nil, errors.New("begin write failed")
}
