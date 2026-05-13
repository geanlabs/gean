package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/geanlabs/gean/api"
	"github.com/geanlabs/gean/checkpoint"
	"github.com/geanlabs/gean/forkchoice"
	"github.com/geanlabs/gean/genesis"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/node"
	"github.com/geanlabs/gean/p2p"
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss"
)

func main() {
	// CLI flags rs L46-79.
	configDir := flag.String("custom-network-config-dir", "", "Config directory (required)")
	gossipPort := flag.Int("gossipsub-port", 9000, "P2P listen port (QUIC/UDP)")
	httpAddr := flag.String("http-address", "127.0.0.1", "Bind address for API + metrics")
	apiPort := flag.Int("api-port", 5052, "API server port")
	metricsPort := flag.Int("metrics-port", 5054, "Metrics server port")
	nodeKey := flag.String("node-key", "", "Path to hex-encoded secp256k1 private key (required)")
	nodeID := flag.String("node-id", "", "Node identifier, e.g. gean_0 (required)")
	checkpointURL := flag.String("checkpoint-sync-url", "", "URL for checkpoint sync (optional)")
	isAggregator := flag.Bool("is-aggregator", false, "Enable attestation aggregation")
	committeeCount := flag.Uint64("attestation-committee-count", 1, "Number of attestation subnets")
	aggregateSubnetIDsStr := flag.String("aggregate-subnet-ids", "", "Comma-separated subnet IDs (requires --is-aggregator)")
	dataDir := flag.String("data-dir", "./data", "Pebble database directory")

	flag.Parse()

	// Validate required flags.
	if *configDir == "" || *nodeKey == "" || *nodeID == "" {
		fmt.Fprintln(os.Stderr, "required flags: --custom-network-config-dir, --node-key, --node-id")
		flag.Usage()
		os.Exit(1)
	}
	if *committeeCount < 1 {
		fmt.Fprintln(os.Stderr, "--attestation-committee-count must be >= 1")
		os.Exit(1)
	}
	if !*isAggregator && *aggregateSubnetIDsStr != "" {
		fmt.Fprintln(os.Stderr, "--aggregate-subnet-ids requires --is-aggregator")
		os.Exit(1)
	}

	logger.Info(logger.Node, "gean consensus client starting")

	// --- Load configuration ---

	configPath := filepath.Join(*configDir, "config.yaml")
	bootnodePath := filepath.Join(*configDir, "nodes.yaml")
	validatorsPath := filepath.Join(*configDir, "annotated_validators.yaml")
	keysDir := filepath.Join(*configDir, "hash-sig-keys")

	genesisConfig, err := genesis.LoadGenesisConfig(configPath)
	if err != nil {
		logger.Error(logger.Node, "load genesis config: %v", err)
		os.Exit(1)
	}
	logger.Info(logger.Node, "genesis: time=%d validators=%d", genesisConfig.GenesisTime, len(genesisConfig.GenesisValidators))

	// Load bootnodes.
	bootnodes, err := p2p.LoadBootnodes(bootnodePath)
	if err != nil {
		logger.Error(logger.Node, "load bootnodes: %v", err)
		os.Exit(1)
	}
	logger.Info(logger.Node, "bootnodes: %d loaded", len(bootnodes))

	// Load validator keys.
	keyManager, err := xmss.LoadValidatorKeys(validatorsPath, keysDir, *nodeID)
	if err != nil {
		logger.Error(logger.Node, "load validator keys: %v", err)
		os.Exit(1)
	}
	defer keyManager.Close()
	logger.Info(logger.Node, "validators: %d keys loaded for %s", len(keyManager.ValidatorIDs()), *nodeID)

	// --- Initialize storage ---

	absDataDir, _ := filepath.Abs(*dataDir)
	os.MkdirAll(absDataDir, 0755)
	logger.Info(logger.Node, "storage: %s", absDataDir)

	backend, err := storage.NewPebbleBackend(absDataDir)
	if err != nil {
		logger.Error(logger.Node, "open pebble: %v", err)
		os.Exit(1)
	}
	defer backend.Close()

	s := node.NewConsensusStore(backend)

	// --- Initialize state (DB restore, checkpoint sync, or genesis) ---

	genesisValidators := genesisConfig.Validators()

	// Check if DB already has a valid head state (restart case).
	existingHead := s.Head()
	existingHeader := s.GetBlockHeader(existingHead)
	existingState := s.GetState(existingHead)

	if existingHeader != nil && existingState != nil && existingHeader.Slot > 0 {
		// DB has valid state — restore from it.
		logger.Info(logger.Node, "restoring from database: slot=%d head=%x justified=%d finalized=%d",
			existingHeader.Slot, existingHead,
			s.LatestJustified().Slot, s.LatestFinalized().Slot)
	} else if *checkpointURL != "" {
		// Checkpoint sync. Per leanSpec PR #713, fetch both the finalized
		// state and the matching SignedBlock and verify the pair via
		// block.state_root == hash_tree_root(state) before trusting the
		// anchor. Storing the SignedBlock lets gean serve /blocks/finalized
		// to the next bootstrap peer without fabricating a synthetic body.
		logger.Info(logger.Sync, "checkpoint sync: %s", *checkpointURL)
		state, signedBlock, err := checkpoint.FetchCheckpointAnchor(*checkpointURL, genesisConfig.GenesisTime, genesisValidators)
		if err != nil {
			logger.Error(logger.Sync, "checkpoint sync failed: %v", err)
			os.Exit(1)
		}
		// initStoreFromState mutates state.LatestBlockHeader.StateRoot from
		// zero (canonical post-state form) to hash_tree_root(state), so the
		// canonical anchor block root must be read from its return value —
		// computing header.HashTreeRoot() before that mutation yields the
		// pre-canonicalization root, which would not match the anchor
		// checkpoint the store sets. Using the wrong key for StorePendingBlock
		// is what caused /lean/v0/blocks/finalized to return 404 on a freshly
		// checkpoint-synced node despite the block being stored.
		canonicalRoot := initStoreFromState(s, state)
		stateRoot := state.LatestBlockHeader.StateRoot
		logger.Info(logger.Sync, "checkpoint sync: slot=%d finalized_root=%x justified_root=%x head_root=%x parent_root=%x state_root=%x",
			state.Slot, state.LatestFinalized.Root, state.LatestJustified.Root, canonicalRoot, state.LatestBlockHeader.ParentRoot, stateRoot)
		s.StorePendingBlock(canonicalRoot, signedBlock)
	} else {
		// Genesis.
		logger.Info(logger.Node, "initializing from genesis")
		genesisState := genesisConfig.GenesisState()
		_ = initStoreFromState(s, genesisState)
	}

	// Rehydrate store.time from wall clock before any consumer reads it.
	// On DB restore the persisted time is stale; on genesis/checkpoint init
	// it is unset. A zero time makes the gossip validator (store_validate.go)
	// reject every attestation as "too far in future" until the first onTick
	// fires, opening an 800ms hole at every boot.
	recoverStoreTime(s, genesisConfig.GenesisTime)

	// --- Initialize fork choice ---

	headRoot := s.Head()
	headHeader := s.GetBlockHeader(headRoot)
	fc := forkchoice.New(headHeader.Slot, headRoot)

	// --- Initialize P2P ---

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Parse explicit aggregate subnet IDs.
	var aggregateSubnetIDs []uint64
	if *aggregateSubnetIDsStr != "" {
		for _, s := range strings.Split(*aggregateSubnetIDsStr, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			id, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid aggregate-subnet-id %q: %v\n", s, err)
				os.Exit(1)
			}
			aggregateSubnetIDs = append(aggregateSubnetIDs, id)
		}
	}

	// Collect validator IDs for subnet subscription.
	var validatorIDs []uint64
	if keyManager != nil {
		validatorIDs = keyManager.ValidatorIDs()
	}

	p2pHost, err := p2p.NewHost(ctx, *nodeKey, *gossipPort, *committeeCount, validatorIDs, *isAggregator, aggregateSubnetIDs)
	if err != nil {
		logger.Error(logger.Network, "create p2p host: %v", err)
		os.Exit(1)
	}
	defer p2pHost.Close()

	logger.Info(logger.Network, "p2p: peer_id=%s listen_port=%d", p2pHost.PeerID(), *gossipPort)

	// Connect to bootnodes.
	p2pHost.ConnectBootnodes(ctx, bootnodes)
	p2pHost.StartBootnodeRedial(ctx, bootnodes)

	// Pre-initialize the XMSS prover so the ~45s setup cost happens before
	// the chain starts, not during the first live aggregation.
	if *isAggregator {
		logger.Info(logger.Node, "pre-initializing XMSS prover (this takes ~45s)...")
		xmss.EnsureProverReady()
		logger.Info(logger.Node, "XMSS prover ready")
	}
	xmss.EnsureVerifierReady()

	// --- Initialize engine ---

	// Runtime-toggleable aggregator role. Seeded from --is-aggregator; the
	// admin API endpoint flips this without restart. Boot-time subscription
	// decisions (p2p.NewHost above, XMSS prover pre-init below) still use
	// the CLI flag per leanSpec PR #636 — only publishing behavior follows
	// the controller at runtime.
	aggCtl := node.NewAggregatorController(*isAggregator)

	n := node.New(s, fc, p2pHost, keyManager, aggCtl, *committeeCount)

	// Register P2P stream handlers.
	p2pHost.RegisterReqRespHandlers(
		func() *p2p.StatusMessage {
			finalized := s.LatestFinalized()
			return &p2p.StatusMessage{
				FinalizedRoot: finalized.Root,
				FinalizedSlot: finalized.Slot,
				HeadRoot:      s.Head(),
				HeadSlot:      s.HeadSlot(),
			}
		},
		func(root [32]byte) *types.SignedBlock {
			return s.GetSignedBlock(root)
		},
		func() uint64 {
			return s.HeadSlot()
		},
		func(startSlot, count uint64) []*types.SignedBlock {
			return s.GetCanonicalBlocksInRange(startSlot, count)
		},
	)

	// Wire gossip handlers — P2P pushes to engine channels.
	p2pHost.StartGossipListeners(n)

	// Start engine goroutine.
	go n.Run(ctx)

	// Start sync driver: periodic status-poll + BlocksByRange backfill when
	// peers are far enough ahead. No-op while node is synced or has no peers.
	// Wire OnPeerConnected as the libp2p PeerStatusHook BEFORE starting the
	// driver so a fast initial dial that completes during start-up still
	// fires the on-connect Status handshake. Sending Status only from the
	// periodic poll is too slow for cold-start single-peer scenarios to
	// learn the peer's head and drive backfill.
	syncDriver := node.NewSyncDriver(ctx, n, p2pHost)
	p2p.PeerStatusHook = syncDriver.OnPeerConnected
	go syncDriver.Run()

	// --- Start HTTP servers ---

	apiAddr := fmt.Sprintf("%s:%d", *httpAddr, *apiPort)
	metricsAddr := fmt.Sprintf("%s:%d", *httpAddr, *metricsPort)

	go func() {
		// Test-driver routes are gated at server-construction time on
		// HIVE_LEAN_TEST_DRIVER=1 (or "true"/"yes"). The hive lean simulator
		// sets this for lean-spec-tests conformance fixtures; production
		// deployments never see it, so the test-driver routes are absent
		// from the mux entirely under normal operation.
		var apiErr error
		if api.IsTestDriverEnabled(os.Getenv(api.TestDriverEnvVar)) {
			logger.Info(logger.Node, "%s=1: enabling test-driver routes", api.TestDriverEnvVar)
			apiErr = api.StartAPIServerWithTestDriver(apiAddr, s, fc, aggCtl)
		} else {
			apiErr = api.StartAPIServer(apiAddr, s, fc, aggCtl)
		}
		if apiErr != nil {
			logger.Error(logger.Node, "api server error: %v", apiErr)
		}
	}()

	go func() {
		if err := api.StartMetricsServer(metricsAddr); err != nil {
			logger.Error(logger.Node, "metrics server error: %v", err)
		}
	}()

	logger.Info(logger.Node, "gean started: api=%s metrics=%s aggregator=%v", apiAddr, metricsAddr, *isAggregator)

	// --- Wait for shutdown ---

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info(logger.Node, "shutting down...")
	cancel()
	// Give engine goroutine time to exit before deferred backend.Close() runs.
	time.Sleep(500 * time.Millisecond)
}

// initStoreFromState initializes the consensus store from an anchor state
// and returns the canonical anchor block root.
//
// The anchor state becomes the new latest justified AND latest finalized
// checkpoint — both pointing at the served block at header.Slot. This
// matches the standard checkpoint sync convention: the bootstrapping node
// trusts the served state as the new finalization anchor and starts forward
// sync from there.
//
// The returned root is the canonical anchor block root — computed AFTER the
// header.StateRoot canonicalization step. Callers that need to associate
// out-of-band data with the anchor block (e.g. StorePendingBlock for the
// checkpoint-sync SignedBlock) must use this return value, not a root
// computed before the function ran; the pre-canonicalization root would not
// match what the store records as latest_finalized.Root.
//
// Note: state.LatestJustified and state.LatestFinalized inside the served
// state point to EARLIER slots (the finalization status from when the block
// was processed). We deliberately do NOT use those — the served block IS
// the new anchor, regardless of what its internal pointers say.
func initStoreFromState(s *node.ConsensusStore, state *types.State) [32]byte {
	// Compute anchor block root from header.
	stateRoot, _ := state.HashTreeRoot()
	header := state.LatestBlockHeader

	// Fill state_root if zero (canonical post-state form from checkpoint server).
	if header.StateRoot == types.ZeroRoot {
		header.StateRoot = stateRoot
	}
	blockRoot, _ := header.HashTreeRoot()

	// Anchor checkpoint: both justified and finalized point at the served block.
	anchor := &types.Checkpoint{Root: blockRoot, Slot: header.Slot}

	// Store metadata.
	s.SetConfig(state.Config)
	s.SetHead(blockRoot)
	s.SetSafeTarget(blockRoot)
	s.SetLatestJustified(anchor)
	s.SetLatestFinalized(anchor)
	// Store time is rehydrated from wall clock by recoverStoreTime after
	// every init path; no need to seed it here.

	// Store block header and state.
	s.InsertBlockHeader(blockRoot, header)
	s.InsertState(blockRoot, state)
	s.InsertLiveChainEntry(state.Slot, blockRoot, header.ParentRoot)

	logger.Info(logger.Store, "store initialized from anchor: slot=%d head=%x parent_root=%x state_root=%x",
		header.Slot, blockRoot, header.ParentRoot, stateRoot)
	return blockRoot
}

// recoverStoreTime sets store.time to the interval index corresponding to the
// current wall clock relative to genesis. Stays at 0 before genesis.
func recoverStoreTime(s *node.ConsensusStore, genesisTimeSec uint64) {
	genesisMs := genesisTimeSec * 1000
	nowMs := uint64(time.Now().UnixMilli())
	if nowMs <= genesisMs {
		s.SetTime(0)
		return
	}
	intervals := (nowMs - genesisMs) / types.MillisecondsPerInterval
	s.SetTime(intervals)
	logger.Info(logger.Node, "store time rehydrated: intervals=%d genesis_time=%d now_ms=%d",
		intervals, genesisTimeSec, nowMs)
}
