package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func openStore(dataDir string) (*storage.PebbleBackend, *store.ConsensusStore, error) {
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve data dir: %w", err)
	}
	if err := os.MkdirAll(absDataDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create data dir: %w", err)
	}
	logger.Info(logger.Node, "storage: %s", absDataDir)

	backend, err := storage.NewPebbleBackend(absDataDir)
	if err != nil {
		return nil, nil, err
	}
	return backend, store.NewConsensusStore(backend), nil
}

func initStoreFromState(s *store.ConsensusStore, state *types.State) ([32]byte, error) {
	if s == nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: store is nil")
	}
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

	logger.Info(logger.Store, "store initialized from anchor: slot=%d head=%x parent_root=%x state_root=%x",
		header.Slot, blockRoot, header.ParentRoot, stateRoot)
	return blockRoot, nil
}

func recoverStoreTime(s *store.ConsensusStore, genesisTimeSec uint64) error {
	if s == nil {
		return fmt.Errorf("recover store time: store is nil")
	}
	if genesisTimeSec > ^uint64(0)/1000 {
		return fmt.Errorf("recover store time: genesis time %d overflows milliseconds", genesisTimeSec)
	}
	genesisMs := genesisTimeSec * 1000
	nowMs := uint64(time.Now().UnixMilli())
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

// forkChoiceFromStore anchors fork choice at the latest justified block and
// replays every stored block above it. FindHead descends from the justified
// root, so after a restart that root must be present in the array — anchoring
// at the bare DB head leaves justified as an unknown ancestor and pins the
// head there permanently. All branches are replayed, not just the head chain:
// the network may have built on a sibling the previous run never chose as
// head, and a missing fork point leaves that subtree dangling and weightless.
func forkChoiceFromStore(s *store.ConsensusStore) (*forkchoice.ForkChoice, error) {
	if s == nil {
		return nil, fmt.Errorf("fork choice anchor: store is nil")
	}
	anchorRoot := s.Head()
	if justified := s.LatestJustified(); justified != nil && s.GetBlockHeader(justified.Root) != nil {
		anchorRoot = justified.Root
	}
	anchorHeader := s.GetBlockHeader(anchorRoot)
	if anchorHeader == nil {
		return nil, fmt.Errorf("fork choice anchor: missing header for root 0x%x", anchorRoot)
	}
	fc := forkchoice.New(anchorHeader.Slot, anchorRoot, anchorHeader.ParentRoot)

	roots, err := s.BlockRoots()
	if err != nil {
		return nil, fmt.Errorf("fork choice anchor: %w", err)
	}
	type replayEntry struct {
		slot         uint64
		root, parent [32]byte
	}
	replay := make([]replayEntry, 0, len(roots))
	for root := range roots {
		header := s.GetBlockHeader(root)
		if header == nil || header.Slot <= anchorHeader.Slot {
			continue
		}
		replay = append(replay, replayEntry{header.Slot, root, header.ParentRoot})
	}
	sort.Slice(replay, func(i, j int) bool { return replay[i].slot < replay[j].slot })
	for _, e := range replay {
		fc.OnBlock(e.slot, e.root, e.parent)
	}
	return fc, nil
}
