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

// makeAttForHead builds a minimal SignedAttestation tagged with a head root,
// used by the engine-level pending-attestation replay tests.
func makeAttForHead(slot uint64, head [32]byte) *types.SignedAttestation {
	return &types.SignedAttestation{
		Data: &types.AttestationData{
			Slot: slot,
			Head: &types.Checkpoint{Root: head},
		},
	}
}
