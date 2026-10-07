package main

import (
	"github.com/geanlabs/gean/consensus/genesis"
	"github.com/geanlabs/gean/crypto/xmss"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/net/p2p"
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
