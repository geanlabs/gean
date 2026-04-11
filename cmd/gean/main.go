package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
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
	_ = flag.String("aggregate-subnet-ids", "", "Comma-separated subnet IDs (requires --is-aggregator)")
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
		// Checkpoint sync.
		logger.Info(logger.Sync, "checkpoint sync: %s", *checkpointURL)
		state, err := checkpoint.FetchCheckpointState(*checkpointURL, genesisConfig.GenesisTime, genesisValidators)
		if err != nil {
			logger.Error(logger.Sync, "checkpoint sync failed: %v", err)
			os.Exit(1)
		}
		stateRoot, _ := state.HashTreeRoot()
		header := state.LatestBlockHeader
		blockRoot, _ := header.HashTreeRoot()
		logger.Info(logger.Sync, "checkpoint sync: slot=%d finalized_root=%x justified_root=%x head_root=%x parent_root=%x state_root=%x",
			state.Slot, state.LatestFinalized.Root, state.LatestJustified.Root, blockRoot, header.ParentRoot, stateRoot)
		initStoreFromState(s, state)
	} else {
		// Genesis.
		logger.Info(logger.Node, "initializing from genesis")
		genesisState := genesisConfig.GenesisState()
		initStoreFromState(s, genesisState)
	}

	// --- Initialize fork choice ---

	headRoot := s.Head()
	headHeader := s.GetBlockHeader(headRoot)
	fc := forkchoice.New(headHeader.Slot, headRoot)

	// --- Initialize P2P ---

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p2pHost, err := p2p.NewHost(ctx, *nodeKey, *gossipPort, *committeeCount)
	if err != nil {
		logger.Error(logger.Network, "create p2p host: %v", err)
		os.Exit(1)
	}
	defer p2pHost.Close()

	logger.Info(logger.Network, "p2p: peer_id=%s listen_port=%d", p2pHost.PeerID(), *gossipPort)

	// Connect to bootnodes.
	p2pHost.ConnectBootnodes(ctx, bootnodes)
	p2pHost.StartBootnodeRedial(ctx, bootnodes)

	// --- Initialize engine ---

	n := node.New(s, fc, p2pHost, keyManager, *isAggregator, *committeeCount)

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
	)

	// Wire gossip handlers — P2P pushes to engine channels.
	p2pHost.StartGossipListeners(n)

	// Start engine goroutine.
	go n.Run(ctx)

	// --- Start HTTP servers ---

	apiAddr := fmt.Sprintf("%s:%d", *httpAddr, *apiPort)
	metricsAddr := fmt.Sprintf("%s:%d", *httpAddr, *metricsPort)

	go func() {
		if err := api.StartAPIServer(apiAddr, s); err != nil {
			logger.Error(logger.Node, "api server error: %v", err)
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

// initStoreFromState initializes the consensus store from an anchor state.
//
// The anchor state becomes the new latest justified AND latest finalized
// checkpoint — both pointing at the served block at header.Slot. This
// matches the standard checkpoint sync convention: the bootstrapping node
// trusts the served state as the new finalization anchor and starts forward
// sync from there.
//
// Note: state.LatestJustified and state.LatestFinalized inside the served
// state point to EARLIER slots (the finalization status from when the block
// was processed). We deliberately do NOT use those — the served block IS
// the new anchor, regardless of what its internal pointers say.
func initStoreFromState(s *node.ConsensusStore, state *types.State) {
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
	s.SetTime(0)

	// Store block header and state.
	s.InsertBlockHeader(blockRoot, header)
	s.InsertState(blockRoot, state)
	s.InsertLiveChainEntry(state.Slot, blockRoot, header.ParentRoot)

	logger.Info(logger.Store, "store initialized from anchor: slot=%d head=%x parent_root=%x state_root=%x",
		header.Slot, blockRoot, header.ParentRoot, stateRoot)
}
