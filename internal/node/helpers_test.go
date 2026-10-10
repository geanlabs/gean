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

func makeAttForHead(slot uint64, head [32]byte) *types.SignedAttestation {
	return &types.SignedAttestation{
		Data: &types.AttestationData{
			Slot: slot,
			Head: &types.Checkpoint{Root: head},
		},
	}
}

// admitPendingBlocks lets bufferMissingParentBlock accept test blocks up to slot:
// the registry gets one validator, so proposer index 0 proposes every slot, and the
// store clock moves to slot.
func admitPendingBlocks(e *Engine, slot uint64) {
	head := e.Store.Head()
	state := e.Store.GetState(head)
	state.Validators = []*types.Validator{{}}
	e.Store.InsertState(head, state)
	e.Store.SetTime(slot * types.IntervalsPerSlot)
}
