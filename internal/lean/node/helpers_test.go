package node

import (
	"github.com/geanlabs/gean/internal/core/storage"
	"github.com/geanlabs/gean/internal/lean/store"
	"github.com/geanlabs/gean/internal/lean/types"
)

func makeTestStore() *store.ConsensusStore {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
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

func blockAtSlot(slot uint64) *types.SignedBlock {
	return &types.SignedBlock{Block: &types.Block{Slot: slot, Body: &types.BlockBody{}}}
}
