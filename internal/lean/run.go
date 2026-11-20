package lean

import (
	"context"
	"fmt"

	"github.com/geanlabs/gean/internal/core/clock"
	"github.com/geanlabs/gean/internal/core/crypto/xmss"
	"github.com/geanlabs/gean/internal/core/httpapi"
	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/lean/importer"
	"github.com/geanlabs/gean/internal/lean/metrics"
	"github.com/geanlabs/gean/internal/lean/node"
	"github.com/geanlabs/gean/internal/lean/role"
	"github.com/geanlabs/gean/internal/lean/shadow"
)

// Run runs a lean node with cfg until ctx ends or a server fails, then shuts
// it down in order.
func Run(ctx context.Context, cfg Config) error {
	logger.Info(logger.Node, "gean consensus client starting")
	stop := ctx.Done()

	// One clock for every time-dependent decision the node makes.
	clk := cfg.Clock
	if clk == nil {
		clk = clock.Real{}
	}

	inputs, err := loadStartupInputs(cfg)
	if err != nil {
		return err
	}
	// Every exit path, including a failure halfway through startup, tears down
	// whatever started, in an order that frees nothing still in use.
	c := &components{freeKeys: inputs.keyManager.Close}
	defer c.teardown()

	// Committee count is a network-wide parameter: resolve the flag against the
	// shared config before any subnet routing is wired (p2p subscriptions and
	// the engine both read cfg.CommitteeCount below).
	cfg.CommitteeCount = resolveCommitteeCount(cfg.CommitteeCount, cfg.CommitteeCountSet, inputs.genesisConfig.AttestationCommitteeCount)
	logger.Info(logger.Node, "attestation committee count: %d", cfg.CommitteeCount)

	backend, s, err := openStore(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("open pebble: %w", err)
	}
	c.closeStore = backend.Close

	if err := bootstrapStore(s, inputs.genesisConfig, cfg.CheckpointURL); err != nil {
		return err
	}

	if err := recoverStoreTime(s, clk, inputs.genesisConfig.GenesisTime); err != nil {
		return err
	}

	fc, err := importer.ForkChoiceFromStore(s)
	if err != nil {
		return err
	}

	// Recover the stored-block high-water mark before any duty runs. Doing it
	// here rather than lazily keeps the full-table scan off the dispatch loop
	// entirely, and surfaces a read failure at startup instead of leaving the
	// duty gate to act on an understated mark.
	if err := s.SeedMaxStoredBlockSlot(); err != nil {
		return fmt.Errorf("seed max stored block slot: %w", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	p2pHost, err := setupP2P(runCtx, cfg, inputs.keyManager)
	if err != nil {
		return fmt.Errorf("create p2p host: %w", err)
	}
	c.closeNetwork = p2pHost.Close

	// Handlers must be live before the prover pre-init: the listener is
	// already accepting connections, and peers that dial during the
	// multi-second init would fail req/resp protocol negotiation.
	registerReqRespHandlers(p2pHost, s, clk)

	proving := len(inputs.keyManager.ValidatorIDs()) > 0 || cfg.IsAggregator
	xmss.SetProverArena(cfg.ProverArena)
	warnIfMemoryLimited(proving)
	if err := preinitializeXMSS(proving); err != nil {
		return err
	}
	// The startup steps above cannot be interrupted, so a signal that arrived
	// during them is honoured here, before the engine starts.
	select {
	case <-stop:
		return nil
	default:
	}

	aggCtl := role.New(cfg.IsAggregator, metrics.SetIsAggregator)
	shadowRates := shadow.Rates{
		AggregateSignatures:        cfg.ShadowAggregateSignaturesRate,
		VerifySignature:            cfg.ShadowVerifySignatureRate,
		VerifyAggregatedSignatures: cfg.ShadowVerifyAggregatedSignaturesRate,
	}
	n := node.New(clk, s, fc, p2pHost, inputs.keyManager, aggCtl, cfg.CommitteeCount, shadowRates)
	n.AggregateSubnetIDs = cfg.AggregateSubnetIDs
	c.waitEngine = n.Wait
	syncDriver := startNodeNetworking(runCtx, n, s, p2pHost, inputs.bootnodes, &c.tasks)
	c.waitSync = syncDriver.Wait

	// After the networking start, so a discovered peer gets the status
	// exchange the sync driver hooks onto new connections.
	if cfg.Discovery {
		if err := p2pHost.StartDiscovery(cfg.DiscoveryPort, cfg.DiscoveryAdvertiseIP, inputs.discoveryBootnodes); err != nil {
			return fmt.Errorf("start discv5: %w", err)
		}
	}

	if c.servers, err = listenHTTP(cfg, s, fc, aggCtl); err != nil {
		return err
	}
	failed := httpapi.Serve(c.servers, &c.tasks)
	logger.Info(logger.Node, "gean started: api=%s metrics=%s aggregator=%v", cfg.apiAddress(), cfg.metricsAddress(), cfg.IsAggregator)

	select {
	case <-stop:
		return nil
	case err := <-failed:
		return err
	}
}
