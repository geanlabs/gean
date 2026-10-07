package node

import (
	"fmt"
	"time"

	"github.com/geanlabs/gean/consensus/forkchoice"
	"github.com/geanlabs/gean/consensus/genesis"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/net/checkpoint"
	"github.com/geanlabs/gean/storage/store"
)

// OpenChain prepares s and its fork choice for an engine. A chain already in
// s is kept; otherwise s starts from the checkpoint at checkpointURL or, with
// none, from genesis. The store clock is set to now, and the stored-block
// high-water mark is recovered so no duty acts on an understated mark.
func OpenChain(s *store.ConsensusStore, genesisConfig *genesis.GenesisConfig, checkpointURL string, now time.Time) (*forkchoice.ForkChoice, error) {
	if err := bootstrapStore(s, genesisConfig, checkpointURL); err != nil {
		return nil, err
	}
	if err := s.RecoverTime(genesisConfig.GenesisTime, now); err != nil {
		return nil, err
	}
	fc, err := ForkChoiceFromStore(s)
	if err != nil {
		return nil, err
	}
	// Recovered here rather than lazily so the full-table scan stays off the
	// dispatch loop and a read failure surfaces at startup.
	if err := s.SeedMaxStoredBlockSlot(); err != nil {
		return nil, fmt.Errorf("seed max stored block slot: %w", err)
	}
	return fc, nil
}

func bootstrapStore(s *store.ConsensusStore, genesisConfig *genesis.GenesisConfig, checkpointURL string) error {
	// Surface the checkpoint-sync configuration up front: a node expected to
	// checkpoint-sync that silently starts from genesis (no url reached the
	// binary) otherwise looks identical to a normal genesis start in the logs.
	if checkpointURL != "" {
		logger.Info(logger.Node, "checkpoint sync configured: url=%s", checkpointURL)
	} else {
		logger.Info(logger.Node, "checkpoint sync not configured (no --checkpoint-sync-url)")
	}

	existingHead := s.Head()
	existingHeader := s.GetBlockHeader(existingHead)
	existingState := s.GetState(existingHead)

	if existingHeader != nil && existingState != nil && existingHeader.Slot > 0 {
		if s.GetSignedBlock(existingHead) == nil {
			return fmt.Errorf("incompatible pre-devnet-5 data directory: signed head block missing; reset the data directory")
		}
		logger.Info(logger.Node, "restoring from database: slot=%d head=%x justified=%d finalized=%d",
			existingHeader.Slot, existingHead,
			s.LatestJustified().Slot, s.LatestFinalized().Slot)
		return nil
	}

	if checkpointURL != "" {
		return bootstrapFromCheckpoint(s, genesisConfig, checkpointURL)
	}

	return bootstrapFromGenesis(s, genesisConfig)
}

func bootstrapFromCheckpoint(s *store.ConsensusStore, genesisConfig *genesis.GenesisConfig, checkpointURL string) error {
	logger.Info(logger.Sync, "checkpoint sync: %s", checkpointURL)
	validators, err := genesisConfig.Validators()
	if err != nil {
		return fmt.Errorf("genesis validators: %w", err)
	}
	state, signedBlock, err := checkpoint.FetchCheckpointAnchor(checkpointURL, genesisConfig.GenesisTime, validators)
	if err != nil {
		logger.Error(logger.Sync, "checkpoint sync failed: %v", err)
		return fmt.Errorf("checkpoint sync failed: %w", err)
	}

	canonicalRoot, err := s.InitFromAnchor(state, signedBlock)
	if err != nil {
		return err
	}
	logger.Info(logger.Sync, "checkpoint sync: slot=%d finalized_root=%x justified_root=%x head_root=%x parent_root=%x state_root=%x",
		state.Slot, state.LatestFinalized.Root, state.LatestJustified.Root, canonicalRoot, state.LatestBlockHeader.ParentRoot, state.LatestBlockHeader.StateRoot)
	return nil
}

func bootstrapFromGenesis(s *store.ConsensusStore, genesisConfig *genesis.GenesisConfig) error {
	logger.Info(logger.Node, "initializing from genesis")
	genesisState, err := genesisConfig.GenesisState()
	if err != nil {
		return fmt.Errorf("build genesis state: %w", err)
	}
	_, err = s.InitFromGenesis(genesisState)
	return err
}
