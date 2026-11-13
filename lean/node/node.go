package node

import (
	"context"
	"net"
	"path/filepath"
	"strconv"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/lean/crypto/xmss"
	"github.com/geanlabs/gean/lean/internal/aggregation/role"
	"github.com/geanlabs/gean/lean/internal/chain"
	"github.com/geanlabs/gean/lean/internal/pools"
	"github.com/geanlabs/gean/lean/internal/prover/shadow"
)

// Config is everything needed to start a Lean node. cmd/gean builds it from
// flags; tests and tools can build it directly.
type Config struct {
	ConfigDir      string
	GossipPort     int
	HTTPAddr       string
	APIPort        int
	MetricsPort    int
	NodeKey        string
	NodeID         string
	CheckpointURL  string
	IsAggregator   bool
	ProverArena    bool
	CommitteeCount uint64
	// CommitteeCountSet reports that CommitteeCount was set explicitly; when
	// false, config.yaml's ATTESTATION_COMMITTEE_COUNT takes precedence.
	CommitteeCountSet  bool
	AggregateSubnetIDs []uint64
	DataDir            string

	// Shadow*Rate model XMSS prover cost for the Shadow network simulator, which
	// does not charge CPU time. Each is in signature-units per second; a
	// non-positive rate (the default) disables the delay, so real deployments are
	// unaffected.
	ShadowAggregateSignaturesRate        float64
	ShadowVerifySignatureRate            float64
	ShadowVerifyAggregatedSignaturesRate float64
}

type configPaths struct {
	config     string
	bootnodes  string
	validators string
	keysDir    string
}

// resolveCommitteeCount picks the effective attestation committee count.
// Precedence is the lean-network convention: an explicit
// --attestation-committee-count flag wins, otherwise the shared config.yaml
// ATTESTATION_COMMITTEE_COUNT, otherwise the flag default (the spec value).
// The loader already guarantees configCount, when present, is >= 1.
func resolveCommitteeCount(flagCount uint64, flagSet bool, configCount *uint64) uint64 {
	if flagSet {
		return flagCount
	}
	if configCount != nil {
		return *configCount
	}
	return flagCount
}

func (c Config) paths() configPaths {
	return configPaths{
		config:     filepath.Join(c.ConfigDir, "config.yaml"),
		bootnodes:  filepath.Join(c.ConfigDir, "nodes.yaml"),
		validators: filepath.Join(c.ConfigDir, "annotated_validators.yaml"),
		keysDir:    filepath.Join(c.ConfigDir, "hash-sig-keys"),
	}
}

func (c Config) apiAddress() string {
	return net.JoinHostPort(c.HTTPAddr, strconv.Itoa(c.APIPort))
}

func (c Config) metricsAddress() string {
	return net.JoinHostPort(c.HTTPAddr, strconv.Itoa(c.MetricsPort))
}

// Run builds the Lean node, runs it until stop closes or a server fails, and
// tears down everything it started, in an order that frees nothing still in use.
func Run(cfg Config, stop <-chan struct{}) error {
	logger.Info(logger.Node, "gean consensus client starting")

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
		logger.Error(logger.Node, "open pebble: %v", err)
		return err
	}
	c.closeStore = backend.Close

	if err := bootstrapStore(s, inputs.genesisConfig, cfg.CheckpointURL); err != nil {
		return err
	}

	if err := recoverStoreTime(s, inputs.genesisConfig.GenesisTime); err != nil {
		return err
	}

	fc, err := chain.ForkChoiceFromStore(s)
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
	c.cancel = cancel

	p2pHost, err := setupP2P(ctx, cfg, inputs.keyManager)
	if err != nil {
		logger.Error(logger.Network, "create p2p host: %v", err)
		return err
	}
	c.closeNetwork = p2pHost.Close

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
	// The startup steps above cannot be interrupted, so a signal that arrived
	// during them is honoured here, before the engine starts.
	select {
	case <-stop:
		return nil
	default:
	}

	aggCtl := role.NewWithHook(cfg.IsAggregator, metrics.SetIsAggregator)
	shadowRates := shadow.Rates{
		AggregateSignatures:        cfg.ShadowAggregateSignaturesRate,
		VerifySignature:            cfg.ShadowVerifySignatureRate,
		VerifyAggregatedSignatures: cfg.ShadowVerifyAggregatedSignaturesRate,
	}
	attestations := pools.New()
	pubKeys := xmss.NewPubKeyCache()
	c.freePubKeys = pubKeys.Close
	n := chain.New(s, attestations, fc, p2pHost, inputs.keyManager, pubKeys, aggCtl, cfg.CommitteeCount, shadowRates)
	n.AggregateSubnetIDs = cfg.AggregateSubnetIDs
	c.waitEngine = n.Wait
	syncDriver := startNodeNetworking(ctx, n, s, p2pHost, inputs.bootnodes, &c.tasks)
	c.waitSync = syncDriver.Wait

	if c.servers, err = listenHTTP(cfg, s, fc, aggCtl); err != nil {
		return err
	}
	failed := serveHTTP(c.servers, &c.tasks)
	logger.Info(logger.Node, "gean started: api=%s metrics=%s aggregator=%v", cfg.apiAddress(), cfg.metricsAddress(), cfg.IsAggregator)

	select {
	case <-stop:
		return nil
	case err := <-failed:
		return err
	}
}
