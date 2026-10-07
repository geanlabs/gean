package main

import (
	"fmt"

	"github.com/geanlabs/gean/checkpoint"
	"github.com/geanlabs/gean/crypto/xmss"
	"github.com/geanlabs/gean/genesis"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/p2p"
	"github.com/geanlabs/gean/store"
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
