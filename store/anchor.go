package store

import (
	"fmt"
	"time"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/types"
)

// InitFromAnchor initializes an empty store at an anchor: the state and the
// signed block it is the post-state of. The anchor becomes head, safe target,
// justified and finalized. It returns the anchor block root.
func (s *ConsensusStore) InitFromAnchor(state *types.State, signedBlock *types.SignedBlock) ([32]byte, error) {
	if state == nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: state is nil")
	}
	header := state.LatestBlockHeader
	if header == nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: latest block header is nil")
	}

	stateRoot, err := state.HashTreeRoot()
	if err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: state root: %w", err)
	}

	if header.StateRoot == types.ZeroRoot {
		header.StateRoot = stateRoot
	}
	blockRoot, err := header.HashTreeRoot()
	if err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: block root: %w", err)
	}

	anchor := &types.Checkpoint{Root: blockRoot, Slot: header.Slot}

	if err := s.PutConfig(state.Config); err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: %w", err)
	}
	if err := s.PutHead(blockRoot); err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: %w", err)
	}
	if err := s.PutSafeTarget(blockRoot); err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: %w", err)
	}
	if err := s.PutLatestJustified(anchor); err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: %w", err)
	}
	if err := s.PutLatestFinalized(anchor); err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: %w", err)
	}
	if err := s.PutBlockHeader(blockRoot, header); err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: %w", err)
	}
	if err := s.PutState(blockRoot, state); err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: %w", err)
	}
	if err := s.PutLiveChainEntry(state.Slot, blockRoot, header.ParentRoot); err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: %w", err)
	}

	if err := s.StorePendingBlock(blockRoot, signedBlock); err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: anchor block: %w", err)
	}

	logger.Info(logger.Store, "store initialized from anchor: slot=%d head=%x parent_root=%x state_root=%x",
		header.Slot, blockRoot, header.ParentRoot, stateRoot)
	return blockRoot, nil
}

// InitFromGenesis initializes an empty store at the genesis state and its
// empty genesis block. It returns the genesis block root.
func (s *ConsensusStore) InitFromGenesis(genesisState *types.State) ([32]byte, error) {
	if genesisState == nil || genesisState.LatestBlockHeader == nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: malformed genesis state")
	}
	header := genesisState.LatestBlockHeader
	// The genesis block commits to the genesis state, so its header must carry
	// the state root before the block is built from it.
	if header.StateRoot == types.ZeroRoot {
		stateRoot, err := genesisState.HashTreeRoot()
		if err != nil {
			return types.ZeroRoot, fmt.Errorf("initialize store: state root: %w", err)
		}
		header.StateRoot = stateRoot
	}
	return s.InitFromAnchor(genesisState, &types.SignedBlock{
		Block: &types.Block{
			Slot:          header.Slot,
			ProposerIndex: header.ProposerIndex,
			ParentRoot:    header.ParentRoot,
			StateRoot:     header.StateRoot,
			Body:          &types.BlockBody{},
		},
		Proof: &types.MultiMessageAggregate{},
	})
}

// RecoverTime sets the store clock to the interval now falls in, counted from
// genesis, or to zero before genesis.
func (s *ConsensusStore) RecoverTime(genesisTimeSec uint64, now time.Time) error {
	if genesisTimeSec > ^uint64(0)/1000 {
		return fmt.Errorf("recover store time: genesis time %d overflows milliseconds", genesisTimeSec)
	}
	genesisMs := genesisTimeSec * 1000
	nowMs := uint64(now.UnixMilli())
	if nowMs <= genesisMs {
		return s.PutTime(0)
	}
	intervals := (nowMs - genesisMs) / types.MillisecondsPerInterval
	if err := s.PutTime(intervals); err != nil {
		return fmt.Errorf("recover store time: %w", err)
	}
	logger.Info(logger.Node, "store time rehydrated: intervals=%d genesis_time=%d now_ms=%d",
		intervals, genesisTimeSec, nowMs)
	return nil
}
