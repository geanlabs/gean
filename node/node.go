package node

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/devylongs/gean/chain"
	"github.com/devylongs/gean/forkchoice"
	"github.com/devylongs/gean/p2p"
	"github.com/devylongs/gean/p2p/reqresp"
	psync "github.com/devylongs/gean/p2p/sync"
	"github.com/devylongs/gean/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

type Node struct {
	config *Config
	store  *forkchoice.Store
	p2p    *p2p.Service
	syncer *psync.Syncer
	logger *slog.Logger

	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	lastProposedSlot types.Slot // Track last slot we proposed/saw a block for
}

type Config struct {
	GenesisTime    uint64
	ValidatorCount uint64
	ValidatorIndex uint64 // types.NoValidator if not a validator
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

	// Generate genesis state
	genesisState := chain.GenerateGenesis(cfg.GenesisTime, cfg.ValidatorCount)

	// Create genesis block
	emptyBody := types.BlockBody{Attestations: []types.SignedVote{}}
	bodyRoot, _ := emptyBody.HashTreeRoot()
	stateRoot, _ := genesisState.HashTreeRoot()

	genesisBlock := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.Root{},
		StateRoot:     stateRoot,
		Body:          emptyBody,
	}

	// Update state with correct block header
	genesisState.LatestBlockHeader = types.BlockHeader{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.Root{},
		StateRoot:     types.Root{}, // Empty for genesis
		BodyRoot:      bodyRoot,
	}

	// Create fork choice store
	store, err := forkchoice.NewStore(genesisState, genesisBlock)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create store: %w", err)
	}

	// Create libp2p host
	host, err := p2p.NewHost(ctx, p2p.HostConfig{
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
	bootnodes, err := p2p.ParseBootnodes(cfg.Bootnodes)
	if err != nil {
		cancel()
		host.Close()
		return nil, fmt.Errorf("parse bootnodes: %w", err)
	}

	// Create p2p service with handlers
	handlers := &p2p.MessageHandlers{
		OnBlock: node.handleBlock,
		OnVote:  node.handleVote,
		Logger:  logger,
	}

	p2pSvc, err := p2p.NewService(ctx, p2p.ServiceConfig{
		Host:      host,
		Handlers:  handlers,
		Bootnodes: bootnodes,
		Logger:    logger,
	})
	if err != nil {
		cancel()
		host.Close()
		return nil, fmt.Errorf("create p2p service: %w", err)
	}

	node.p2p = p2pSvc

	// Create request/response handler
	reqrespHandler := reqresp.NewHandler(store)

	// Create stream handler and register protocols
	streamHandler := reqresp.NewStreamHandler(host, reqrespHandler)
	streamHandler.RegisterProtocols()

	// Create syncer for chain synchronization
	syncer := psync.NewSyncer(ctx, psync.Config{
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
	n.p2p.Start()
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
	n.p2p.Stop()
	n.logger.Info("node stopped")
}

func (n *Node) slotTicker() {
	defer n.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.C:
			n.onTick()
		}
	}
}

func (n *Node) onTick() {
	currentTime := uint64(time.Now().Unix())

	// Don't do anything before genesis
	if currentTime < n.config.GenesisTime {
		return
	}

	n.store.AdvanceTime(currentTime, false)

	slot := n.store.CurrentSlot()
	interval := n.store.CurrentInterval()

	// Log slot progression at start of each slot
	if interval == 0 {
		n.logger.Debug("slot", "slot", slot, "head", n.store.Head[:4], "peers", n.PeerCount())
	}

	if n.config.ValidatorIndex == types.NoValidator {
		return
	}

	// Interval 0: Proposer produces block (skip slot 0 - that's genesis)
	if interval == 0 && slot > 0 {
		if slot <= n.lastProposedSlot {
			return
		}
		proposerIndex := uint64(slot) % n.config.ValidatorCount
		if proposerIndex == n.config.ValidatorIndex {
			n.lastProposedSlot = slot
			n.proposeBlock(slot)
		}
	}

	// Interval 1: Validators vote (skip slot 0 - no block to vote on yet)
	if interval == 1 && slot > 0 {
		n.produceVote(slot)
	}
}

func (n *Node) handleBlock(ctx context.Context, signedBlock *types.SignedBlock, from peer.ID) error {
	block := &signedBlock.Message

	// First, check if we need to request missing parent blocks
	if err := n.syncer.OnBlockReceived(signedBlock, from); err != nil {
		n.logger.Warn("failed to request parent blocks", "error", err)
	}

	// Try to process the block
	if err := n.store.ProcessBlock(block); err != nil {
		// If parent state not found, it might be due to missing parent blocks
		if strings.Contains(err.Error(), "parent") {
			return fmt.Errorf("process block: %w (parent sync may be in progress)", err)
		}
		return fmt.Errorf("process block: %w", err)
	}

	// Track that we've seen a block for this slot to prevent proposing for same slot
	if block.Slot > n.lastProposedSlot {
		n.lastProposedSlot = block.Slot
	}

	n.logger.Info("processed block",
		"slot", block.Slot,
		"proposer", block.ProposerIndex,
	)
	return nil
}

// handleVote processes an incoming vote from the network.
func (n *Node) handleVote(ctx context.Context, vote *types.SignedVote) error {
	if err := n.store.ProcessAttestation(vote); err != nil {
		return fmt.Errorf("process vote: %w", err)
	}
	n.logger.Debug("processed vote",
		"slot", vote.Data.Slot,
		"validator", vote.Data.ValidatorID,
	)
	return nil
}

func (n *Node) proposeBlock(slot types.Slot) {
	validatorIndex := types.ValidatorIndex(n.config.ValidatorIndex)

	// ProduceBlock iteratively collects attestations and computes state root
	block, err := n.store.ProduceBlock(slot, validatorIndex)
	if err != nil {
		n.logger.Warn("produce block failed", "slot", slot, "error", err)
		return
	}

	// Create signed block (signature is placeholder for Devnet 0)
	signedBlock := &types.SignedBlock{
		Message:   *block,
		Signature: types.Root{},
	}

	if err := n.p2p.PublishBlock(n.ctx, signedBlock); err != nil {
		n.logger.Error("failed to publish block", "slot", slot, "error", err)
		return
	}

	n.logger.Info("proposed block", "slot", slot, "attestations", len(block.Body.Attestations))
}

func (n *Node) produceVote(slot types.Slot) {
	validatorIndex := types.ValidatorIndex(n.config.ValidatorIndex)

	// Use ProduceAttestationVote which handles locking correctly
	voteData := n.store.ProduceAttestationVote(slot, validatorIndex)

	vote := &types.SignedVote{
		Data:      *voteData,
		Signature: types.Root{}, // Placeholder signature
	}

	if err := n.p2p.PublishVote(n.ctx, vote); err != nil {
		n.logger.Error("failed to publish vote", "slot", slot, "error", err)
		return
	}

	// Process our own vote
	if err := n.store.ProcessAttestation(vote); err != nil {
		n.logger.Error("failed to process own vote", "slot", slot, "error", err)
		return
	}

	n.logger.Debug("produced vote", "slot", slot)
}

// CurrentSlot returns the current slot.
func (n *Node) CurrentSlot() types.Slot {
	return n.store.CurrentSlot()
}

// PeerCount returns the number of connected peers.
func (n *Node) PeerCount() int {
	return n.p2p.PeerCount()
}
