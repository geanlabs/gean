// Package sync implements the synchronization protocol for the Lean consensus client.
package sync

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/devylongs/gean/forkchoice"
	"github.com/devylongs/gean/p2p/reqresp"
	"github.com/devylongs/gean/types"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

// SyncState represents the current synchronization state.
type SyncState int

const (
	SyncStateIdle SyncState = iota
	SyncStateSyncing
	SyncStateSynced
)

// PeerStatus holds a peer's last known status.
type PeerStatus struct {
	Status    *types.Status
	UpdatedAt time.Time
}

// Syncer manages synchronization with peers.
type Syncer struct {
	host          host.Host
	store         *forkchoice.Store
	streamHandler *reqresp.StreamHandler
	reqrespHandler *reqresp.Handler
	logger        *slog.Logger

	mu          sync.RWMutex
	peerStatus  map[peer.ID]*PeerStatus
	state       SyncState

	// Pending parent requests to avoid duplicate requests
	pendingParents   map[types.Root]struct{}
	pendingParentsMu sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
}

// Config holds syncer configuration.
type Config struct {
	Host           host.Host
	Store          *forkchoice.Store
	StreamHandler  *reqresp.StreamHandler
	ReqRespHandler *reqresp.Handler
	Logger         *slog.Logger
}

// NewSyncer creates a new syncer.
func NewSyncer(ctx context.Context, cfg Config) *Syncer {
	ctx, cancel := context.WithCancel(ctx)

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Syncer{
		host:           cfg.Host,
		store:          cfg.Store,
		streamHandler:  cfg.StreamHandler,
		reqrespHandler: cfg.ReqRespHandler,
		logger:         logger,
		peerStatus:     make(map[peer.ID]*PeerStatus),
		pendingParents: make(map[types.Root]struct{}),
		state:          SyncStateIdle,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start begins the syncer background tasks.
func (s *Syncer) Start() {
	// Register connection notifier
	notifier := NewConnectionNotifier(s, s.logger)
	s.host.Network().Notify(notifier)

	// Check for existing peers (e.g., bootnodes connected before syncer started)
	for _, peerID := range s.host.Network().Peers() {
		s.logger.Debug("found existing peer, initiating status exchange", "peer", peerID)
		go func(pid peer.ID) {
			ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
			defer cancel()
			if err := s.InitiateStatusExchange(ctx, pid); err != nil {
				s.logger.Warn("status exchange with existing peer failed",
					"peer", pid,
					"error", err,
				)
			}
		}(peerID)
	}

	// Start periodic sync check
	go s.syncLoop()

	s.logger.Info("syncer started")
}

// Stop shuts down the syncer.
func (s *Syncer) Stop() {
	s.cancel()
	s.logger.Info("syncer stopped")
}

// InitiateStatusExchange sends our status and processes peer's response.
func (s *Syncer) InitiateStatusExchange(ctx context.Context, peerID peer.ID) error {
	ourStatus := reqresp.NewStatus(s.store)

	s.logger.Debug("sending status to peer",
		"peer", peerID,
		"head_slot", ourStatus.Head.Slot,
		"finalized_slot", ourStatus.Finalized.Slot,
	)

	peerStatus, err := s.streamHandler.SendStatus(ctx, peerID, ourStatus)
	if err != nil {
		return fmt.Errorf("send status: %w", err)
	}

	return s.processPeerStatus(peerID, peerStatus)
}

// processPeerStatus validates and stores peer status, triggers sync if needed.
func (s *Syncer) processPeerStatus(peerID peer.ID, peerStatus *types.Status) error {
	s.logger.Debug("received peer status",
		"peer", peerID,
		"peer_head_slot", peerStatus.Head.Slot,
		"peer_finalized_slot", peerStatus.Finalized.Slot,
	)

	// Validate peer status
	if err := s.reqrespHandler.ValidatePeerStatus(peerStatus); err != nil {
		s.logger.Warn("invalid peer status, disconnecting",
			"peer", peerID,
			"error", err,
		)
		// Close connection to peer with conflicting finalized checkpoint
		s.host.Network().ClosePeer(peerID)
		return err
	}

	// Store peer status
	s.mu.Lock()
	s.peerStatus[peerID] = &PeerStatus{
		Status:    peerStatus,
		UpdatedAt: time.Now(),
	}
	s.mu.Unlock()

	// Check if we need to sync
	ourStatus := reqresp.NewStatus(s.store)
	if peerStatus.Head.Slot > ourStatus.Head.Slot {
		s.logger.Info("peer ahead, initiating sync",
			"peer", peerID,
			"peer_head_slot", peerStatus.Head.Slot,
			"our_head_slot", ourStatus.Head.Slot,
		)
		go s.syncFromPeer(peerID, peerStatus)
	}

	return nil
}

// syncFromPeer requests missing blocks from a peer.
func (s *Syncer) syncFromPeer(peerID peer.ID, peerStatus *types.Status) {
	s.mu.Lock()
	if s.state == SyncStateSyncing {
		s.mu.Unlock()
		return // Already syncing
	}
	s.state = SyncStateSyncing
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.state = SyncStateIdle
		s.mu.Unlock()

		// After sync completes, advance store time to current wall clock
		// This ensures the node knows what slot it's actually at
		currentTime := uint64(time.Now().Unix())
		s.store.AdvanceTime(currentTime, false)
	}()

	// Request the peer's head block first
	roots := []types.Root{peerStatus.Head.Root}

	s.logger.Debug("requesting blocks from peer",
		"peer", peerID,
		"roots", len(roots),
	)

	blocks, err := s.streamHandler.RequestBlocksByRoot(s.ctx, peerID, roots)
	if err != nil {
		s.logger.Warn("failed to request blocks",
			"peer", peerID,
			"error", err,
		)
		return
	}

	s.logger.Debug("received blocks from peer",
		"peer", peerID,
		"count", len(blocks),
	)

	for _, block := range blocks {
		if err := s.processReceivedBlock(block, peerID); err != nil {
			s.logger.Warn("failed to process block",
				"slot", block.Message.Slot,
				"error", err,
			)
		}
	}
}

// processReceivedBlock processes a block received via req/resp.
// If parent is unknown, requests parent chain.
func (s *Syncer) processReceivedBlock(block *types.SignedBlock, fromPeer peer.ID) error {
	// Check if we already have this block
	blockRoot, err := block.Message.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("hash block: %w", err)
	}

	s.store.RLock()
	_, exists := s.store.Blocks[blockRoot]
	s.store.RUnlock()

	if exists {
		return nil // Already have this block
	}

	// Check if parent exists
	parentRoot := block.Message.ParentRoot

	s.store.RLock()
	_, parentExists := s.store.Blocks[parentRoot]
	s.store.RUnlock()

	if !parentExists {
		// Parent unknown - request parent chain
		if err := s.requestParentChain(parentRoot, fromPeer); err != nil {
			return fmt.Errorf("request parent chain: %w", err)
		}
	}

	// Process the block
	if err := s.store.ProcessBlock(&block.Message); err != nil {
		return fmt.Errorf("process block: %w", err)
	}

	s.logger.Info("synced block",
		"slot", block.Message.Slot,
		"proposer", block.Message.ProposerIndex,
	)

	return nil
}

// requestParentChain requests missing parent blocks recursively.
func (s *Syncer) requestParentChain(parentRoot types.Root, fromPeer peer.ID) error {
	// Check if we're already requesting this parent
	s.pendingParentsMu.Lock()
	if _, pending := s.pendingParents[parentRoot]; pending {
		s.pendingParentsMu.Unlock()
		return nil // Already requesting this parent
	}
	s.pendingParents[parentRoot] = struct{}{}
	s.pendingParentsMu.Unlock()

	defer func() {
		s.pendingParentsMu.Lock()
		delete(s.pendingParents, parentRoot)
		s.pendingParentsMu.Unlock()
	}()

	s.logger.Debug("requesting parent block",
		"root", fmt.Sprintf("%x", parentRoot[:4]),
		"peer", fromPeer,
	)

	blocks, err := s.streamHandler.RequestBlocksByRoot(s.ctx, fromPeer, []types.Root{parentRoot})
	if err != nil {
		return fmt.Errorf("request parent: %w", err)
	}

	for _, block := range blocks {
		if err := s.processReceivedBlock(block, fromPeer); err != nil {
			s.logger.Warn("failed to process parent block",
				"slot", block.Message.Slot,
				"error", err,
			)
		}
	}

	return nil
}

// RemovePeer removes a peer from tracking.
func (s *Syncer) RemovePeer(peerID peer.ID) {
	s.mu.Lock()
	delete(s.peerStatus, peerID)
	s.mu.Unlock()
}

// syncLoop periodically checks sync status and requests updates.
func (s *Syncer) syncLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkSyncStatus()
		}
	}
}

// checkSyncStatus compares our status with known peers.
func (s *Syncer) checkSyncStatus() {
	ourStatus := reqresp.NewStatus(s.store)

	s.mu.RLock()
	var bestPeer peer.ID
	var bestSlot types.Slot
	var bestStatus *types.Status

	for peerID, ps := range s.peerStatus {
		if ps.Status.Head.Slot > bestSlot && ps.Status.Head.Slot > ourStatus.Head.Slot {
			bestPeer = peerID
			bestSlot = ps.Status.Head.Slot
			bestStatus = ps.Status
		}
	}
	s.mu.RUnlock()

	if bestPeer != "" && bestStatus != nil {
		go s.syncFromPeer(bestPeer, bestStatus)
	}
}

// OnBlockReceived is called when a block is received via gossip.
// Checks for unknown parent and triggers parent request if needed.
func (s *Syncer) OnBlockReceived(block *types.SignedBlock, fromPeer peer.ID) error {
	parentRoot := block.Message.ParentRoot

	s.store.RLock()
	_, exists := s.store.Blocks[parentRoot]
	s.store.RUnlock()

	if !exists {
		// Unknown parent - request it
		return s.requestParentChain(parentRoot, fromPeer)
	}

	return nil
}

// GetSyncState returns the current sync state.
func (s *Syncer) GetSyncState() SyncState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// IsSynced returns true if we believe we are synced with the network.
func (s *Syncer) IsSynced() bool {
	ourStatus := reqresp.NewStatus(s.store)

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ps := range s.peerStatus {
		if ps.Status.Head.Slot > ourStatus.Head.Slot+1 {
			return false
		}
	}

	return true
}
