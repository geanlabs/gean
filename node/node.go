// Package node implements the top-level Lean Ethereum consensus node.
//
// The node orchestrates all subsystems:
//   - consensus: state transition functions (injected into the store)
//   - forkchoice: block tree, vote tracking, LMD-GHOST head selection
//   - networking: gossipsub for blocks/votes, req/resp for chain sync
//   - validator: block production and vote creation
//
// The node runs a 1-second ticker that drives slot progression. At each tick:
//   - Interval 0: proposer produces a block (if assigned)
//   - Interval 1: all validators produce attestation votes
//   - Interval 2-3: handled internally by the store (safe target, vote acceptance)
package node

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/devylongs/gean/consensus"
	"github.com/devylongs/gean/forkchoice"
	"github.com/devylongs/gean/networking"
	"github.com/devylongs/gean/networking/chainsync"
	"github.com/devylongs/gean/networking/reqresp"
	"github.com/devylongs/gean/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Node is the top-level consensus client that connects all subsystems.
type Node struct {
	config *Config
	store  *forkchoice.Store
	net    *networking.Service
	syncer *chainsync.Syncer
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
	syncer := chainsync.NewSyncer(ctx, chainsync.Config{
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

// onTick is called every second to drive the slot pipeline.
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
		finalized := n.store.GetLatestFinalized()
		n.logger.Debug("slot",
			"slot", slot,
			"head", n.store.GetHead().Short(),
			"justified", n.store.GetLatestJustified().Slot,
			"finalized", finalized.Slot,
			"peers", n.PeerCount(),
		)
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

	// Interval 1: Validators attest (skip slot 0 - no block to attest on yet)
	if interval == 1 && slot > 0 {
		// Proposer already includes and processes its attestation at interval 0.
		proposerIndex := uint64(slot) % n.config.ValidatorCount
		if proposerIndex == n.config.ValidatorIndex {
			return
		}
		n.produceAttestation(slot)
	}
}

// handleBlock processes an incoming block from the network.
func (n *Node) handleBlock(ctx context.Context, signed *types.SignedBlockWithAttestation, from peer.ID) error {
	block := &signed.Message.Block

	// First, check if we need to request missing parent blocks
	if err := n.syncer.OnBlockReceived(signed, from); err != nil {
		n.logger.Warn("failed to request parent blocks", "error", err)
	}

	// Try to process the block
	if err := n.store.ProcessBlock(block); err != nil {
		// If parent not found, it might be due to missing parent blocks (sync in progress)
		if errors.Is(err, forkchoice.ErrParentNotFound) {
			return fmt.Errorf("%w: %v", ErrSyncInProgress, err)
		}
		return fmt.Errorf("process block: %w", err)
	}

	// Process proposer attestation as a pending (gossip-stage) attestation.
	// This happens after head update inside ProcessBlock to avoid circular weight.
	proposerSigned := &types.SignedAttestation{
		Message: signed.Message.ProposerAttestation,
	}
	if err := n.store.ProcessAttestation(proposerSigned); err != nil {
		n.logger.Warn("failed to process proposer attestation from block envelope",
			"slot", block.Slot,
			"validator", proposerSigned.Message.ValidatorID,
			"error", err,
		)
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

// handleAttestation processes an incoming attestation from the network.
func (n *Node) handleAttestation(ctx context.Context, att *types.SignedAttestation) error {
	if err := n.store.ProcessAttestation(att); err != nil {
		return fmt.Errorf("process attestation: %w", err)
	}
	n.logger.Debug("processed attestation",
		"slot", att.Message.Data.Slot,
		"validator", att.Message.ValidatorID,
	)
	return nil
}

// proposeBlock produces and publishes a block for the given slot.
// Uses the iterative attestation collection algorithm (see forkchoice.Store.ProduceBlock).
func (n *Node) proposeBlock(slot types.Slot) {
	validatorIndex := types.ValidatorIndex(n.config.ValidatorIndex)

	// ProduceBlock iteratively collects attestations and computes state root
	block, err := n.store.ProduceBlock(slot, validatorIndex)
	if err != nil {
		n.logger.Warn("produce block failed", "slot", slot, "error", err)
		return
	}

	// Build proposer attestation data
	attData := n.store.ProduceAttestationData(slot)
	proposerAtt := types.Attestation{
		ValidatorID: n.config.ValidatorIndex,
		Data:        *attData,
	}

	// Create signed block envelope.
	// TODO: attach XMSS signatures once key management is implemented.
	signedBlock := &types.SignedBlockWithAttestation{
		Message: types.BlockWithAttestation{
			Block:               *block,
			ProposerAttestation: proposerAtt,
		},
		// Signature list is empty until XMSS signing is wired.
	}

	// Process proposer attestation locally as pending gossip-stage vote.
	// Proposers skip interval-1 attestation to avoid double-attesting.
	proposerSigned := &types.SignedAttestation{Message: proposerAtt}
	if err := n.store.ProcessAttestation(proposerSigned); err != nil {
		n.logger.Warn("failed to process own proposer attestation",
			"slot", slot,
			"validator", proposerSigned.Message.ValidatorID,
			"error", err,
		)
	}

	if err := n.net.PublishBlock(n.ctx, signedBlock); err != nil {
		n.logger.Error("failed to publish block", "slot", slot, "error", err)
		return
	}

	n.logger.Info("proposed block", "slot", slot, "attestations", len(block.Body.Attestations))
}

// produceAttestation creates and publishes an attestation, then processes it locally.
func (n *Node) produceAttestation(slot types.Slot) {
	validatorIndex := types.ValidatorIndex(n.config.ValidatorIndex)

	// Produce attestation data
	attData := n.store.ProduceAttestationData(slot)

	att := &types.SignedAttestation{
		Message: types.Attestation{
			ValidatorID: uint64(validatorIndex),
			Data:        *attData,
		},
		// Signature is zero until XMSS signing is wired.
	}

	if err := n.net.PublishAttestation(n.ctx, att); err != nil {
		n.logger.Error("failed to publish attestation", "slot", slot, "error", err)
		return
	}

	// Process our own attestation
	if err := n.store.ProcessAttestation(att); err != nil {
		n.logger.Error("failed to process own attestation", "slot", slot, "error", err)
		return
	}

	n.logger.Debug("produced attestation", "slot", slot)
}

// CurrentSlot returns the current slot.
func (n *Node) CurrentSlot() types.Slot {
	return n.store.CurrentSlot()
}

// PeerCount returns the number of connected peers.
func (n *Node) PeerCount() int {
	return n.net.PeerCount()
}
