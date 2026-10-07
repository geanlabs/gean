package main

import (
	"context"
	"errors"
	"flag"
	"os"

	"github.com/geanlabs/gean/crypto/xmss"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/metrics"
	"github.com/geanlabs/gean/node"
	"github.com/geanlabs/gean/role"
	"github.com/geanlabs/gean/shadow"
	"github.com/geanlabs/gean/tasks"
)

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
	// shared config before any subnet routing is wired (p2p subscriptions and
	// the engine both read cfg.CommitteeCount below).
	cfg.CommitteeCount = resolveCommitteeCount(cfg.CommitteeCount, cfg.committeeCountSet, inputs.genesisConfig.AttestationCommitteeCount)
	logger.Info(logger.Node, "attestation committee count: %d", cfg.CommitteeCount)

	backend, s, err := openStore(cfg.DataDir)
	if err != nil {
		logger.Error(logger.Node, "open pebble: %v", err)
		return err
	}
	defer backend.Close()

	if err := bootstrapStore(s, inputs.genesisConfig, cfg.CheckpointURL); err != nil {
		return err
	}

	if err := recoverStoreTime(s, inputs.genesisConfig.GenesisTime); err != nil {
		return err
	}

	fc, err := node.ForkChoiceFromStore(s)
	if err != nil {
		return err
	}

	// Recover the stored-block high-water mark before any duty runs. Doing it
	// here rather than lazily keeps the full-table scan off the dispatch loop
	// entirely, and surfaces a read failure at startup instead of leaving the
	// duty gate to act on an understated mark.
	if err := s.SeedMaxStoredBlockSlot(); err != nil {
		logger.Error(logger.Node, "seed max stored block slot: %v", err)
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p2pHost, err := setupP2P(ctx, cfg, inputs.keyManager)
	if err != nil {
		logger.Error(logger.Network, "create p2p host: %v", err)
		return err
	}
	defer p2pHost.Close()

	// Handlers must be live before the prover pre-init: the listener is
	// already accepting connections, and peers that dial during the
	// multi-second init would fail req/resp protocol negotiation.
	registerReqRespHandlers(p2pHost, s)

	proving := len(inputs.keyManager.ValidatorIDs()) > 0 || cfg.IsAggregator
	xmss.SetProverArena(cfg.ProverArena)
	warnIfMemoryLimited(proving)
	if err := preinitializeXMSS(proving); err != nil {
		return err
	}

	aggCtl := role.NewWithHook(cfg.IsAggregator, metrics.SetIsAggregator)
	shadowRates := shadow.Rates{
		AggregateSignatures:        cfg.ShadowAggregateSignaturesRate,
		VerifySignature:            cfg.ShadowVerifySignatureRate,
		VerifyAggregatedSignatures: cfg.ShadowVerifyAggregatedSignaturesRate,
	}
	pubKeys := xmss.NewPubKeyCache()
	defer pubKeys.Close()
	n := node.New(node.Components{
		Store:      s,
		ForkChoice: fc,
		P2P:        p2pHost,
		Keys:       inputs.keyManager,
		PubKeys:    pubKeys,
		Aggregator: aggCtl,
	}, node.Config{
		CommitteeCount:     cfg.CommitteeCount,
		AggregateSubnetIDs: cfg.AggregateSubnetIDs,
		Shadow:             shadowRates,
	})

	// services owns the engine, sync driver and HTTP servers. Each returns only
	// after its own work has finished, so once services.Wait returns the
	// deferred closes can run: the p2p host (which joins its handlers), then
	// storage, then the keys.
	var services tasks.Group
	startNodeNetworking(ctx, &services, n, s, p2pHost, inputs.bootnodes)

	apiAddr, metricsAddr := startHTTPServers(ctx, &services, cfg, s, fc, aggCtl)
	logger.Info(logger.Node, "gean started: api=%s metrics=%s aggregator=%v", apiAddr, metricsAddr, cfg.IsAggregator)

	waitForShutdown(cancel)
	services.Wait()
	return nil
}
