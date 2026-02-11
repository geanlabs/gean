// lifecycle.go contains node wiring, construction, and lifecycle entrypoints.
//
// Package node implements the top-level Lean Ethereum consensus node.
//
// The node orchestrates all subsystems:
//   - consensus: state transition functions (injected into the store)
//   - forkchoice: block tree, vote tracking, LMD-GHOST head selection
//   - networking: gossipsub for blocks/votes, req/resp for chain sync
//   - validator: block production and vote creation
package node

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/devylongs/gean/consensus"
	"github.com/devylongs/gean/forkchoice"
	"github.com/devylongs/gean/networking"
	"github.com/devylongs/gean/networking/reqresp"
	netsync "github.com/devylongs/gean/networking/sync"
	"github.com/devylongs/gean/types"
)

// Node is the top-level consensus client that connects all subsystems.
type Node struct {
	config *Config
	store  *forkchoice.Store
	net    *networking.Service
	syncer *netsync.Syncer
	logger *slog.Logger

	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	lastProposedSlot types.Slot // Track last slot we proposed/saw a block for
}

type Config struct {
	GenesisTime    uint64
	ValidatorCount uint64
	ValidatorIndex uint64
	ListenAddrs    []string
	Bootnodes      []string
	Logger         *slog.Logger
}

// New creates a new node with the given configuration.
func New(ctx context.Context, cfg *Config) (*Node, error) {
	ctx, cancel := context.WithCancel(ctx)

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Generate deterministic placeholder validators for genesis.
	// Real XMSS key loading and signing are added in later phases.
	validators := consensus.GenerateValidators(int(cfg.ValidatorCount))
	genesisState, genesisBlock := consensus.GenerateGenesis(cfg.GenesisTime, validators)

	// Create fork choice store with injected state transition functions
	store, err := forkchoice.NewStore(genesisState, genesisBlock, consensus.ProcessSlots, consensus.ProcessBlock, forkchoice.WithLogger(logger))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create store: %w", err)
	}

	// Create libp2p host
	host, err := networking.NewHost(ctx, networking.HostConfig{
		ListenAddrs: cfg.ListenAddrs,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create host: %w", err)
	}

	node := &Node{
		config: cfg,
		store:  store,
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}

	// Parse bootnodes
	bootnodes, err := networking.ParseBootnodes(cfg.Bootnodes)
	if err != nil {
		cancel()
		host.Close()
		return nil, fmt.Errorf("parse bootnodes: %w", err)
	}

	// Create networking service with handlers
	handlers := &networking.MessageHandlers{
		OnBlock:       node.handleBlock,
		OnAttestation: node.handleAttestation,
	}

	netSvc, err := networking.NewService(ctx, networking.ServiceConfig{
		Host:      host,
		Handlers:  handlers,
		Bootnodes: bootnodes,
		Logger:    logger,
	})
	if err != nil {
		cancel()
		host.Close()
		return nil, fmt.Errorf("create networking service: %w", err)
	}

	node.net = netSvc

	// Create request/response handler
	reqrespHandler := reqresp.NewHandler(store)

	// Create stream handler and register protocols
	streamHandler := reqresp.NewStreamHandler(host, reqrespHandler)
	streamHandler.RegisterProtocols()

	// Create syncer for chain synchronization
	syncer := netsync.NewSyncer(ctx, netsync.Config{
		Host:           host,
		Store:          store,
		StreamHandler:  streamHandler,
		ReqRespHandler: reqrespHandler,
		Logger:         logger,
	})
	node.syncer = syncer

	return node, nil
}

// Start begins node operation.
func (n *Node) Start() {
	n.net.Start()
	n.syncer.Start()

	n.wg.Add(1)
	go n.slotTicker()

	n.logger.Info("node started",
		"genesis_time", n.config.GenesisTime,
		"validators", n.config.ValidatorCount,
	)
}

// Stop gracefully shuts down the node.
func (n *Node) Stop() {
	n.cancel()
	n.wg.Wait()
	n.syncer.Stop()
	n.net.Stop()
	n.logger.Info("node stopped")
}

// CurrentSlot returns the current slot.
func (n *Node) CurrentSlot() types.Slot {
	return n.store.CurrentSlot()
}

// PeerCount returns the number of connected peers.
func (n *Node) PeerCount() int {
	return n.net.PeerCount()
}
