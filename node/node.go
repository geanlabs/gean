package node

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/devylongs/gean/consensus"
	"github.com/devylongs/gean/p2p"
)

// Node is the main consensus client that orchestrates all components.
type Node struct {
	config *Config
	store  *consensus.Store
	p2p    *p2p.Service
	logger *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Config holds node configuration.
type Config struct {
	GenesisTime    uint64
	ValidatorCount uint64
	ValidatorIndex *uint64 // nil if not a validator
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
	genesisState := consensus.GenerateGenesis(cfg.GenesisTime, cfg.ValidatorCount)

	// Create genesis block
	emptyBody := consensus.BlockBody{Attestations: []consensus.SignedVote{}}
	bodyRoot, _ := emptyBody.HashTreeRoot()
	stateRoot, _ := genesisState.HashTreeRoot()

	genesisBlock := &consensus.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    consensus.Root{},
		StateRoot:     stateRoot,
		Body:          emptyBody,
	}

	// Update state with correct block header
	genesisState.LatestBlockHeader = consensus.BlockHeader{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    consensus.Root{},
		StateRoot:     consensus.Root{}, // Empty for genesis
		BodyRoot:      bodyRoot,
	}

	// Create fork choice store
	store, err := consensus.NewStore(genesisState, genesisBlock)
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
		OnBlock:       node.handleBlock,
		OnAttestation: node.handleAttestation,
		Logger:        logger,
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

	return node, nil
}

// Start begins node operation.
func (n *Node) Start() {
	n.p2p.Start()

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
	n.p2p.Stop()
	n.logger.Info("node stopped")
}

// slotTicker runs the slot-based event loop.
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

// onTick is called every second to advance time and check duties.
func (n *Node) onTick() {
	currentTime := uint64(time.Now().Unix())
	n.store.AdvanceTime(currentTime, false)

	slot := n.store.CurrentSlot()
	interval := n.currentInterval()

	// Interval 0: Proposer produces block
	if interval == 0 && n.config.ValidatorIndex != nil {
		headState := n.store.States[n.store.Head]
		if headState != nil && headState.IsProposer(consensus.ValidatorIndex(*n.config.ValidatorIndex)) {
			n.proposeBlock(slot)
		}
	}

	// Interval 2: Validators vote
	if interval == 2 && n.config.ValidatorIndex != nil {
		n.produceVote(slot)
	}
}

// currentInterval returns the current interval within the slot (0-3).
func (n *Node) currentInterval() uint64 {
	return n.store.Time % consensus.IntervalsPerSlot
}

// handleBlock processes an incoming block from the network.
func (n *Node) handleBlock(ctx context.Context, signedBlock *consensus.SignedBlock) error {
	block := &signedBlock.Message
	if err := n.store.ProcessBlock(block); err != nil {
		return fmt.Errorf("process block: %w", err)
	}
	n.logger.Info("processed block",
		"slot", block.Slot,
		"proposer", block.ProposerIndex,
	)
	return nil
}

// handleAttestation processes an incoming attestation from the network.
func (n *Node) handleAttestation(ctx context.Context, vote *consensus.SignedVote) error {
	n.store.ProcessAttestation(vote)
	n.logger.Debug("processed attestation",
		"slot", vote.Data.Slot,
		"validator", vote.Data.ValidatorID,
	)
	return nil
}

// proposeBlock creates and publishes a new block.
func (n *Node) proposeBlock(slot consensus.Slot) {
	parentRoot := n.store.GetProposalHead(slot)
	parentState := n.store.States[parentRoot]
	if parentState == nil {
		n.logger.Warn("parent state not found", "slot", slot)
		return
	}

	// Process slots to current
	newState, err := parentState.ProcessSlots(slot)
	if err != nil {
		n.logger.Warn("process slots failed", "slot", slot, "error", err)
		return
	}

	// Create block
	body := consensus.BlockBody{Attestations: []consensus.SignedVote{}}

	block := &consensus.Block{
		Slot:          slot,
		ProposerIndex: *n.config.ValidatorIndex,
		ParentRoot:    parentRoot,
		StateRoot:     consensus.Root{}, // Will be updated
		Body:          body,
	}

	// Apply block to get state root
	postState, err := newState.ProcessBlock(block)
	if err != nil {
		n.logger.Warn("process block failed", "slot", slot, "error", err)
		return
	}
	stateRoot, _ := postState.HashTreeRoot()
	block.StateRoot = stateRoot

	// Create signed block (signature is placeholder)
	signedBlock := &consensus.SignedBlock{
		Message:   *block,
		Signature: consensus.Root{}, // Placeholder signature
	}

	if err := n.p2p.PublishBlock(n.ctx, signedBlock); err != nil {
		n.logger.Error("failed to publish block", "slot", slot, "error", err)
		return
	}

	// Process our own block
	if err := n.store.ProcessBlock(block); err != nil {
		n.logger.Error("failed to process own block", "slot", slot, "error", err)
		return
	}

	n.logger.Info("proposed block", "slot", slot)
}

// produceVote creates and publishes a vote.
func (n *Node) produceVote(slot consensus.Slot) {
	target := n.store.GetVoteTarget()
	head := n.store.Head
	headBlock := n.store.Blocks[head]

	vote := &consensus.SignedVote{
		Data: consensus.Vote{
			Slot:        slot,
			ValidatorID: *n.config.ValidatorIndex,
			Head:        consensus.Checkpoint{Root: head, Slot: headBlock.Slot},
			Target:      target,
			Source:      n.store.LatestJustified,
		},
		Signature: consensus.Root{}, // Placeholder signature
	}

	if err := n.p2p.PublishAttestation(n.ctx, vote); err != nil {
		n.logger.Error("failed to publish vote", "slot", slot, "error", err)
		return
	}

	// Process our own vote
	n.store.ProcessAttestation(vote)

	n.logger.Debug("produced vote", "slot", slot)
}

// CurrentSlot returns the current slot.
func (n *Node) CurrentSlot() consensus.Slot {
	return n.store.CurrentSlot()
}

// Head returns the current head root.
func (n *Node) Head() consensus.Root {
	return n.store.Head
}

// PeerCount returns the number of connected peers.
func (n *Node) PeerCount() int {
	return n.p2p.PeerCount()
}
