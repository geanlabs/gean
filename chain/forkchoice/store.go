package forkchoice

import (
	"fmt"
	"sync"

	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

var log = logging.NewComponentLogger(logging.CompForkChoice)

// Store tracks chain state and validator votes for the LMD GHOST algorithm.
type Store struct {
	mu sync.Mutex

	Time          uint64
	GenesisTime   uint64
	NumValidators uint64
	Head          [32]byte
	SafeTarget    [32]byte

	LatestJustified *types.Checkpoint
	LatestFinalized *types.Checkpoint
	Storage         storage.Store

	LatestKnownAttestations map[uint64]*types.SignedAttestation
	LatestNewAttestations   map[uint64]*types.SignedAttestation
}

// ChainStatus is a snapshot of the fork choice head and checkpoint state.
type ChainStatus struct {
	Head          [32]byte
	HeadSlot      uint64
	JustifiedSlot uint64
	FinalizedSlot uint64
	FinalizedRoot [32]byte
}

// GetStatus returns a consistent snapshot of the chain head and checkpoints.
func (c *Store) GetStatus() ChainStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	headSlot := uint64(0)
	if hb, ok := c.Storage.GetBlock(c.Head); ok {
		headSlot = hb.Slot
	}
	return ChainStatus{
		Head:          c.Head,
		HeadSlot:      headSlot,
		JustifiedSlot: c.LatestJustified.Slot,
		FinalizedSlot: c.LatestFinalized.Slot,
		FinalizedRoot: c.LatestFinalized.Root,
	}
}

// NewStore initializes a store from an anchor state and block.
func NewStore(state *types.State, anchorBlock *types.Block, store storage.Store) *Store {
	stateRoot, _ := state.HashTreeRoot()
	if anchorBlock.StateRoot != stateRoot {
		panic(fmt.Sprintf("anchor block state root mismatch: block=%x state=%x", anchorBlock.StateRoot, stateRoot))
	}

	anchorRoot, _ := anchorBlock.HashTreeRoot()

	store.PutBlock(anchorRoot, anchorBlock)
	store.PutSignedBlock(anchorRoot, &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{Block: anchorBlock},
	})
	store.PutState(anchorRoot, state)

	return &Store{
		Time:                    anchorBlock.Slot * types.SecondsPerSlot,
		GenesisTime:             state.Config.GenesisTime,
		NumValidators:           uint64(len(state.Validators)),
		Head:                    anchorRoot,
		SafeTarget:              anchorRoot,
		LatestJustified:         state.LatestJustified,
		LatestFinalized:         state.LatestFinalized,
		Storage:                 store,
		LatestKnownAttestations: make(map[uint64]*types.SignedAttestation),
		LatestNewAttestations:   make(map[uint64]*types.SignedAttestation),
	}
}
