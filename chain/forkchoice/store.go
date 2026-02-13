package forkchoice

import (
	"fmt"
	"sync"

	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

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

	LatestKnownAttestations map[uint64]*types.Attestation
	LatestNewAttestations   map[uint64]*types.Attestation
}

// NewStore initializes a store from an anchor state and block.
func NewStore(state *types.State, anchorBlock *types.Block, store storage.Store) *Store {
	stateRoot, _ := state.HashTreeRoot()
	if anchorBlock.StateRoot != stateRoot {
		panic(fmt.Sprintf("anchor block state root mismatch: block=%x state=%x", anchorBlock.StateRoot, stateRoot))
	}

	anchorRoot, _ := anchorBlock.HashTreeRoot()

	store.PutBlock(anchorRoot, anchorBlock)
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
		LatestKnownAttestations: make(map[uint64]*types.Attestation),
		LatestNewAttestations:   make(map[uint64]*types.Attestation),
	}
}
