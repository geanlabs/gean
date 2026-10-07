package store

import (
	"fmt"
	"time"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/storage/db"
	"github.com/geanlabs/gean/types"
)

// InitFromAnchor initializes an empty store at an anchor: the state and the
// signed block it is the post-state of. The anchor becomes head, safe target,
// justified and finalized. Everything is serialized first and written in one
// batch, so a failure leaves the store untouched rather than holding a head
// whose block is missing. It returns the anchor block root.
func (s *ConsensusStore) InitFromAnchor(state *types.State, signedBlock *types.SignedBlock) ([32]byte, error) {
	if state == nil || state.LatestBlockHeader == nil || state.Config == nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: malformed anchor state")
	}
	if signedBlock == nil || signedBlock.Block == nil || signedBlock.Block.Body == nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: malformed anchor block")
	}
	header := state.LatestBlockHeader

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

	block := signedBlock.Block
	bodyRoot, err := block.Body.HashTreeRoot()
	if err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: body root: %w", err)
	}
	blockHeader := &types.BlockHeader{
		Slot:          block.Slot,
		ProposerIndex: block.ProposerIndex,
		ParentRoot:    block.ParentRoot,
		StateRoot:     block.StateRoot,
		BodyRoot:      bodyRoot,
	}
	anchor := &types.Checkpoint{Root: blockRoot, Slot: header.Slot}

	configData, err := state.Config.MarshalSSZ()
	if err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: marshal config: %w", err)
	}
	anchorData, err := anchor.MarshalSSZ()
	if err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: marshal anchor: %w", err)
	}
	headerData, err := blockHeader.MarshalSSZ()
	if err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: marshal header: %w", err)
	}
	stateData, err := state.MarshalSSZ()
	if err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: marshal state: %w", err)
	}
	bodyData, err := block.Body.MarshalSSZ()
	if err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: marshal body: %w", err)
	}
	blockData, err := signedBlock.MarshalSSZ()
	if err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: marshal signed block: %w", err)
	}

	wb, err := s.beginWrite("initialize store")
	if err != nil {
		return types.ZeroRoot, err
	}
	// One batch is atomic, so the order of its puts does not matter.
	entries := map[db.Table][]db.KV{
		db.TableMetadata: {
			{Key: db.KeyConfig, Value: configData},
			{Key: db.KeyHead, Value: blockRoot[:]},
			{Key: db.KeySafeTarget, Value: blockRoot[:]},
			{Key: db.KeyLatestJustified, Value: anchorData},
			{Key: db.KeyLatestFinalized, Value: anchorData},
		},
		db.TableBlockHeaders: {{Key: blockRoot[:], Value: headerData}},
		db.TableStates:       {{Key: blockRoot[:], Value: stateData}},
		db.TableLiveChain:    {{Key: db.EncodeLiveChainKey(state.Slot, blockRoot), Value: header.ParentRoot[:]}},
		db.TableSignedBlocks: {{Key: blockRoot[:], Value: blockData}},
	}
	if len(bodyData) > 0 {
		entries[db.TableBlockBodies] = []db.KV{{Key: blockRoot[:], Value: bodyData}}
	}
	for table, kvs := range entries {
		if err := wb.PutBatch(table, kvs); err != nil {
			return types.ZeroRoot, fmt.Errorf("initialize store: put %s: %w", table, err)
		}
	}
	if err := wb.Commit(); err != nil {
		return types.ZeroRoot, fmt.Errorf("initialize store: commit: %w", err)
	}
	s.ObserveStoredBlockSlot(max(header.Slot, block.Slot))

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
