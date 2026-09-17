package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/geanlabs/gean/internal/execution"
	"github.com/geanlabs/gean/internal/execution/embedded"
	"github.com/geanlabs/gean/internal/genesis"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/node"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// executionStartupTimeout bounds the handshake and genesis check.
const executionStartupTimeout = 10 * time.Second

// setupExecution pairs the node with an execution client, embedded or
// remote, when the network declares one. It returns the function that stops
// the client at shutdown. Both modes run the same checks: the client must
// advertise the engine methods gean calls, and its genesis must be the
// network's.
func setupExecution(ctx context.Context, cfg config, genesisConfig *genesis.GenesisConfig, s *store.ConsensusStore, n *node.Engine, validatorCount int) (func(), error) {
	expectedGenesis, hasExecutionLayer := genesisConfig.ExecutionGenesisBlockHash()
	noop := func() {}

	if cfg.ELGenesis == "" && cfg.ExecutionEndpoint == "" {
		if hasExecutionLayer && validatorCount > 0 {
			return nil, fmt.Errorf("network declares an execution layer (EXECUTION_GENESIS_BLOCK_HASH); validators need --el-genesis or --execution-endpoint")
		}
		if hasExecutionLayer {
			logger.Warn(logger.Execution, "no execution client configured: payloads are imported without execution-layer validation")
		}
		return noop, nil
	}
	if !hasExecutionLayer {
		return nil, fmt.Errorf("an execution client was configured but config.yaml declares no EXECUTION_GENESIS_BLOCK_HASH")
	}

	engine, closer, where, err := startEngine(cfg)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, executionStartupTimeout)
	defer cancel()

	supported, err := engine.ExchangeCapabilities(ctx, execution.Capabilities)
	if err != nil {
		closer()
		return nil, fmt.Errorf("execution client handshake (%s): %w", where, err)
	}
	for _, method := range execution.Capabilities {
		if !contains(supported, method) {
			logger.Warn(logger.Execution, "execution client does not advertise %s", method)
		}
	}

	actualGenesis, err := engine.GenesisBlockHash(ctx)
	if err != nil {
		closer()
		return nil, fmt.Errorf("read execution genesis block: %w", err)
	}
	if actualGenesis != expectedGenesis {
		closer()
		return nil, fmt.Errorf("execution genesis 0x%x does not match EXECUTION_GENESIS_BLOCK_HASH 0x%x; it is a different chain",
			actualGenesis, expectedGenesis)
	}
	metrics.SetExecutionReachable(true)

	if cfg.FeeRecipient == ([types.AddressSize]byte{}) && validatorCount > 0 {
		logger.Warn(logger.Execution, "--suggested-fee-recipient not set; block rewards go to the zero address")
	}

	n.Execution = node.NewExecutionDriver(engine, s, cfg.FeeRecipient)
	logger.Info(logger.Execution, "execution client paired: %s genesis=0x%x", where, actualGenesis)
	return closer, nil
}

// startEngine builds the configured execution client. Embedded mode runs
// geth in this process from a genesis file; remote mode connects to a
// client's authenticated engine port.
func startEngine(cfg config) (execution.Engine, func(), string, error) {
	if cfg.ELGenesis != "" {
		genesis, err := embedded.LoadGenesis(cfg.ELGenesis)
		if err != nil {
			return nil, nil, "", err
		}
		engine, err := embedded.Start(embedded.Config{
			Genesis:   genesis,
			DataDir:   filepath.Join(cfg.DataDir, "el"),
			HTTPPort:  cfg.ELHTTPPort,
			P2PPort:   cfg.ELP2PPort,
			Bootnodes: cfg.ELBootnodes,
		})
		if err != nil {
			return nil, nil, "", err
		}
		if cfg.ELP2PPort > 0 {
			logger.Info(logger.Execution, "execution p2p enode=%s", engine.Enode())
		}
		return engine, func() { _ = engine.Close() }, "embedded geth from " + cfg.ELGenesis, nil
	}

	secret, err := execution.LoadJWTSecret(cfg.ExecutionJWTSecret)
	if err != nil {
		return nil, nil, "", err
	}
	client, err := execution.NewClient(cfg.ExecutionEndpoint, secret)
	if err != nil {
		return nil, nil, "", err
	}
	return client, func() {}, "endpoint=" + cfg.ExecutionEndpoint, nil
}

func contains(list []string, item string) bool {
	for _, entry := range list {
		if entry == item {
			return true
		}
	}
	return false
}
