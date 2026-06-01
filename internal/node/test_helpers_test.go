package node

import (
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func makeTestStore() *store.ConsensusStore {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	s.SetConfig(&types.ChainConfig{GenesisTime: 1000})
	return s
}
