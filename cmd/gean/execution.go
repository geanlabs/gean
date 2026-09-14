package main

import (
	"context"
	"fmt"
	"time"

	"github.com/geanlabs/gean/internal/execution"
	"github.com/geanlabs/gean/internal/genesis"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/node"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// executionStartupTimeout bounds the handshake and genesis check together. A
// client that is up answers both in milliseconds; one that is still starting
// gets a few seconds before the mismatch is reported.
const executionStartupTimeout = 10 * time.Second

// setupExecution pairs the node with an execution client when the network
// declares one. It refuses configurations that cannot work: an endpoint on a
// network without an execution layer, validator keys on an execution network
// without an endpoint, or a client whose genesis is not the network's.
func setupExecution(ctx context.Context, cfg config, genesisConfig *genesis.GenesisConfig, s *store.ConsensusStore, n *node.Engine, validatorCount int) error {
	expectedGenesis, hasExecutionLayer := genesisConfig.ExecutionGenesisBlockHash()

	if cfg.ExecutionEndpoint == "" {
		if hasExecutionLayer && validatorCount > 0 {
			return fmt.Errorf("network declares an execution layer (EXECUTION_GENESIS_BLOCK_HASH); validators need --execution-endpoint and --execution-jwt-secret")
		}
		if hasExecutionLayer {
			logger.Warn(logger.Execution, "no --execution-endpoint: payloads are imported without execution-layer validation")
		}
		return nil
	}
	if !hasExecutionLayer {
		return fmt.Errorf("--execution-endpoint given but config.yaml declares no EXECUTION_GENESIS_BLOCK_HASH")
	}

	secret, err := execution.LoadJWTSecret(cfg.ExecutionJWTSecret)
	if err != nil {
		return err
	}
	client := execution.NewClient(cfg.ExecutionEndpoint, secret)

	ctx, cancel := context.WithTimeout(ctx, executionStartupTimeout)
	defer cancel()

	supported, err := client.ExchangeCapabilities(ctx, execution.Capabilities)
	if err != nil {
		return fmt.Errorf("execution client handshake at %s: %w", cfg.ExecutionEndpoint, err)
	}
	for _, method := range execution.Capabilities {
		if !contains(supported, method) {
			logger.Warn(logger.Execution, "execution client does not advertise %s", method)
		}
	}

	actualGenesis, err := client.GenesisBlockHash(ctx)
	if err != nil {
		return fmt.Errorf("read execution genesis block: %w", err)
	}
	if actualGenesis != expectedGenesis {
		return fmt.Errorf("execution client genesis 0x%x does not match EXECUTION_GENESIS_BLOCK_HASH 0x%x; it is running a different chain",
			actualGenesis, expectedGenesis)
	}
	metrics.SetExecutionReachable(true)

	if cfg.FeeRecipient == ([types.AddressSize]byte{}) && validatorCount > 0 {
		logger.Warn(logger.Execution, "--suggested-fee-recipient not set; block rewards go to the zero address")
	}

	n.Execution = node.NewExecutionDriver(client, s, cfg.FeeRecipient)
	logger.Info(logger.Execution, "execution client paired: endpoint=%s genesis=0x%x", cfg.ExecutionEndpoint, actualGenesis)
	return nil
}

func contains(list []string, item string) bool {
	for _, entry := range list {
		if entry == item {
			return true
		}
	}
	return false
}
