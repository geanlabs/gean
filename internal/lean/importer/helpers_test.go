package importer

import (
	"os"
	"testing"

	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/core/pending"
	"github.com/geanlabs/gean/internal/core/storage"
	"github.com/geanlabs/gean/internal/lean/forkchoice"
	"github.com/geanlabs/gean/internal/lean/store"
	"github.com/geanlabs/gean/internal/lean/types"
)

func TestMain(m *testing.M) {
	logger.SetQuiet(true)
	os.Exit(m.Run())
}

func makeTestStore() *store.ConsensusStore {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	s.SetConfig(&types.ChainConfig{GenesisTime: 1000})
	return s
}

// makeTestImporter returns an importer over a store holding only genesis.
func makeTestImporter() *Importer {
	s := makeTestStore()
	var genesisRoot [32]byte
	genesisRoot[0] = 0x01
	s.SetHead(genesisRoot)
	s.SetSafeTarget(genesisRoot)
	s.SetLatestJustified(&types.Checkpoint{Root: genesisRoot, Slot: 0})
	s.SetLatestFinalized(&types.Checkpoint{Root: genesisRoot, Slot: 0})
	s.InsertBlockHeader(genesisRoot, &types.BlockHeader{Slot: 0})
	s.InsertState(genesisRoot, &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     0,
		LatestBlockHeader:        &types.BlockHeader{},
		LatestJustified:          &types.Checkpoint{Root: genesisRoot, Slot: 0},
		LatestFinalized:          &types.Checkpoint{Root: genesisRoot, Slot: 0},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	})
	return New(Config{
		Store:               s,
		FC:                  forkchoice.New(0, genesisRoot, [32]byte{}),
		PendingAttestations: pending.NewAttestationBuffer(8, 512, types.AttestationSlot),
	})
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
func admitPendingBlocks(imp *Importer, slot uint64) {
	head := imp.store.Head()
	state := imp.store.GetState(head)
	state.Validators = []*types.Validator{{}}
	imp.store.InsertState(head, state)
	imp.store.SetTime(slot * types.IntervalsPerSlot)
}

// stateFinalizing builds an SSZ-serializable post-state pinned to the given
// finalized checkpoint.
func stateFinalizing(slot uint64, finalized *types.Checkpoint) *types.State {
	return &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     slot,
		LatestBlockHeader:        &types.BlockHeader{},
		LatestJustified:          finalized,
		LatestFinalized:          finalized,
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
}
