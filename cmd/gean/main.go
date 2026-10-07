package main

import (
	"context"
	"errors"
	"flag"
	"os"

	"github.com/geanlabs/gean/launch"
	"github.com/geanlabs/gean/logger"
)

// gitCommit is injected at build time with -ldflags "-X main.gitCommit=...".
var gitCommit = "unknown"

func main() {
	cfg, err := parseConfig(os.Args[1:], os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		os.Exit(1)
	}
	if err := run(cfg); err != nil {
		logger.Error(logger.Node, "fatal: %v", err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	logger.Info(logger.Node, "gean consensus client starting")

	inputs, err := loadStartupInputs(cfg)
	if err != nil {
		return err
	}
	defer inputs.keyManager.Close()

	// Committee count is a network-wide parameter: resolve the flag against the
	// shared config before any subnet routing is wired.
	cfg.CommitteeCount = resolveCommitteeCount(cfg.CommitteeCount, cfg.committeeCountSet, inputs.genesisConfig.AttestationCommitteeCount)
	logger.Info(logger.Node, "attestation committee count: %d", cfg.CommitteeCount)

	n, err := launch.Launch(context.Background(), launch.Config{
		DataDir:            cfg.DataDir,
		Genesis:            inputs.genesisConfig,
		CheckpointURL:      cfg.CheckpointURL,
		Bootnodes:          inputs.bootnodes,
		Keys:               inputs.keyManager,
		NodeKeyPath:        cfg.NodeKey,
		GossipPort:         cfg.GossipPort,
		APIAddress:         cfg.apiAddress(),
		MetricsAddress:     cfg.metricsAddress(),
		CommitteeCount:     cfg.CommitteeCount,
		AggregateSubnetIDs: cfg.AggregateSubnetIDs,
		IsAggregator:       cfg.IsAggregator,
		ProverArena:        cfg.ProverArena,
		Shadow:             cfg.shadowRates(),
		GitCommit:          gitCommit,
		APIHandler:         apiHandler,
	})
	if err != nil {
		return err
	}

	go stopOnSignal(n)
	err = n.Wait()
	if errors.Is(err, launch.ErrShutdownTimeout) {
		// Services still hold storage and keys, so nothing may be released:
		// exit without running the deferred closes.
		logger.Error(logger.Node, "fatal: %v", err)
		os.Exit(1)
	}
	return err
}
