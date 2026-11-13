package validator_test

import (
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/lean/consensus/types"
	"github.com/geanlabs/gean/lean/internal/store"
)

func makeValidationStore() *store.ConsensusStore {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	s.SetConfig(&types.ChainConfig{GenesisTime: 1000})
	return s
}
