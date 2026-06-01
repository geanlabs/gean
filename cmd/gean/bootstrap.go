package main

import (
	"fmt"

	"github.com/geanlabs/gean/internal/checkpoint"
	"github.com/geanlabs/gean/internal/genesis"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/p2p"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
	"github.com/multiformats/go-multiaddr"
)

type startupInputs struct {
	genesisConfig *genesis.GenesisConfig
	bootnodes     []multiaddr.Multiaddr
	keyManager    *xmss.KeyManager
}

func loadStartupInputs(cfg config) (*startupInputs, error) {
	paths := cfg.paths()

	genesisConfig, err := genesis.LoadGenesisConfig(paths.config)
	if err != nil {
		logger.Error(logger.Node, "load genesis config: %v", err)
		return nil, err
	}
	logger.Info(logger.Node, "genesis: time=%d validators=%d", genesisConfig.GenesisTime, len(genesisConfig.GenesisValidators))

	bootnodes, err := p2p.LoadBootnodes(paths.bootnodes)
	if err != nil {
		logger.Error(logger.Node, "load bootnodes: %v", err)
		return nil, err
	}
	logger.Info(logger.Node, "bootnodes: %d loaded", len(bootnodes))

	keyManager, err := xmss.LoadValidatorKeys(paths.validators, paths.keysDir, cfg.NodeID)
	if err != nil {
		logger.Error(logger.Node, "load validator keys: %v", err)
		return nil, err
	}
	logger.Info(logger.Node, "validators: %d keys loaded for %s", len(keyManager.ValidatorIDs()), cfg.NodeID)

	return &startupInputs{
		genesisConfig: genesisConfig,
		bootnodes:     bootnodes,
		keyManager:    keyManager,
	}, nil
}

func bootstrapStore(s *store.ConsensusStore, genesisConfig *genesis.GenesisConfig, checkpointURL string) error {
	existingHead := s.Head()
	existingHeader := s.GetBlockHeader(existingHead)
	existingState := s.GetState(existingHead)

	if existingHeader != nil && existingState != nil && existingHeader.Slot > 0 {
		logger.Info(logger.Node, "restoring from database: slot=%d head=%x justified=%d finalized=%d",
			existingHeader.Slot, existingHead,
			s.LatestJustified().Slot, s.LatestFinalized().Slot)
		return nil
	}

	if checkpointURL != "" {
		return bootstrapFromCheckpoint(s, genesisConfig, checkpointURL)
	}

	bootstrapFromGenesis(s, genesisConfig)
	return nil
}

func bootstrapFromCheckpoint(s *store.ConsensusStore, genesisConfig *genesis.GenesisConfig, checkpointURL string) error {
	logger.Info(logger.Sync, "checkpoint sync: %s", checkpointURL)
	state, signedBlock, err := checkpoint.FetchCheckpointAnchor(checkpointURL, genesisConfig.GenesisTime, genesisConfig.Validators())
	if err != nil {
		logger.Error(logger.Sync, "checkpoint sync failed: %v", err)
		return fmt.Errorf("checkpoint sync failed: %w", err)
	}

	// Use initStoreFromState's RETURN value as the anchor block root: it
	// canonicalizes header.StateRoot first, so a root computed before this
	// call would not match what the store records as latest_finalized.Root —
	// which previously caused /lean/v0/blocks/finalized to 404. See the
	// initStoreFromState doc in store.go for the full rationale.
	canonicalRoot := initStoreFromState(s, state)
	stateRoot := state.LatestBlockHeader.StateRoot
	logger.Info(logger.Sync, "checkpoint sync: slot=%d finalized_root=%x justified_root=%x head_root=%x parent_root=%x state_root=%x",
		state.Slot, state.LatestFinalized.Root, state.LatestJustified.Root, canonicalRoot, state.LatestBlockHeader.ParentRoot, stateRoot)
	s.StorePendingBlock(canonicalRoot, signedBlock)
	return nil
}

func bootstrapFromGenesis(s *store.ConsensusStore, genesisConfig *genesis.GenesisConfig) {
	logger.Info(logger.Node, "initializing from genesis")
	genesisState := genesisConfig.GenesisState()
	canonicalRoot := initStoreFromState(s, genesisState)
	genesisSignedBlock := &types.SignedBlock{
		Block: &types.Block{
			Slot:          genesisState.LatestBlockHeader.Slot,
			ProposerIndex: genesisState.LatestBlockHeader.ProposerIndex,
			ParentRoot:    genesisState.LatestBlockHeader.ParentRoot,
			StateRoot:     genesisState.LatestBlockHeader.StateRoot,
			Body:          &types.BlockBody{},
		},
		Signature: &types.BlockSignatures{
			ProposerSignature: types.BlankXMSSSignature(),
		},
	}
	s.StorePendingBlock(canonicalRoot, genesisSignedBlock)
}
