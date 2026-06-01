package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func openStore(dataDir string) (*storage.PebbleBackend, *store.ConsensusStore, error) {
	absDataDir, _ := filepath.Abs(dataDir)
	os.MkdirAll(absDataDir, 0755)
	logger.Info(logger.Node, "storage: %s", absDataDir)

	backend, err := storage.NewPebbleBackend(absDataDir)
	if err != nil {
		return nil, nil, err
	}
	return backend, store.NewConsensusStore(backend), nil
}

// initStoreFromState initializes the consensus store from an anchor state
// and returns the canonical anchor block root.
//
// The anchor state becomes the new latest justified AND latest finalized
// checkpoint — both pointing at the served block at header.Slot. This
// matches the standard checkpoint sync convention: the bootstrapping node
// trusts the served state as the new finalization anchor and starts forward
// sync from there.
//
// The returned root is the canonical anchor block root — computed AFTER the
// header.StateRoot canonicalization step. Callers that need to associate
// out-of-band data with the anchor block (e.g. StorePendingBlock for the
// checkpoint-sync SignedBlock) must use this return value, not a root
// computed before the function ran; the pre-canonicalization root would not
// match what the store records as latest_finalized.Root.
//
// Note: state.LatestJustified and state.LatestFinalized inside the served
// state point to EARLIER slots (the finalization status from when the block
// was processed). We deliberately do NOT use those — the served block IS
// the new anchor, regardless of what its internal pointers say.
func initStoreFromState(s *store.ConsensusStore, state *types.State) [32]byte {
	stateRoot, _ := state.HashTreeRoot()
	header := state.LatestBlockHeader

	if header.StateRoot == types.ZeroRoot {
		header.StateRoot = stateRoot
	}
	blockRoot, _ := header.HashTreeRoot()

	anchor := &types.Checkpoint{Root: blockRoot, Slot: header.Slot}

	s.SetConfig(state.Config)
	s.SetHead(blockRoot)
	s.SetSafeTarget(blockRoot)
	s.SetLatestJustified(anchor)
	s.SetLatestFinalized(anchor)
	s.InsertBlockHeader(blockRoot, header)
	s.InsertState(blockRoot, state)
	s.InsertLiveChainEntry(state.Slot, blockRoot, header.ParentRoot)

	logger.Info(logger.Store, "store initialized from anchor: slot=%d head=%x parent_root=%x state_root=%x",
		header.Slot, blockRoot, header.ParentRoot, stateRoot)
	return blockRoot
}

// recoverStoreTime sets store.time to the interval index corresponding to the
// current wall clock relative to genesis. Stays at 0 before genesis.
func recoverStoreTime(s *store.ConsensusStore, genesisTimeSec uint64) {
	genesisMs := genesisTimeSec * 1000
	nowMs := uint64(time.Now().UnixMilli())
	if nowMs <= genesisMs {
		s.SetTime(0)
		return
	}
	intervals := (nowMs - genesisMs) / types.MillisecondsPerInterval
	s.SetTime(intervals)
	logger.Info(logger.Node, "store time rehydrated: intervals=%d genesis_time=%d now_ms=%d",
		intervals, genesisTimeSec, nowMs)
}
