package node

import (
	"github.com/geanlabs/gean/storage/db"
	"github.com/geanlabs/gean/storage/store"
	"github.com/geanlabs/gean/types"
)

func makeTestStore() *store.ConsensusStore {
	s := store.NewConsensusStore(db.NewInMemoryBackend())
	s.SetConfig(&types.ChainConfig{GenesisTime: 1000})
	return s
}

func makeAttForHead(slot uint64, head [32]byte) *types.SignedAttestation {
	return &types.SignedAttestation{
		Data: &types.AttestationData{
			Slot: slot,
			Head: &types.Checkpoint{Root: head},
		},
	}
}
