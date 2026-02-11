// Package chainsync implements the chain synchronization protocol for the Lean consensus client.
//
// When a node discovers a peer with a higher head slot (via the Status handshake),
// it requests missing blocks via the BlocksByRoot req/resp protocol and processes
// them in parent-first order. Missing parents are fetched recursively.
//
// Sync requests use exponential backoff retry (1s, 2s, 4s, max 3 retries) to
// handle transient stream failures gracefully.
package chainsync

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/devylongs/gean/networking/reqresp"
	"github.com/devylongs/gean/types"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// ChainStore provides access to the block store for chain synchronization.
// Satisfied by forkchoice.Store without modification.
type ChainStore interface {
	HasBlock(root types.Root) bool
	ProcessBlock(block *types.Block) error
	AdvanceTime(unixTime uint64, hasProposal bool)
}

const (
	reqrespTimeout  = 30 * time.Second
	maxSyncRetries  = 3
	baseRetryDelay  = 1 * time.Second
)

type SyncState int

const (
	SyncStateIdle SyncState = iota
	SyncStateSyncing
)

type Syncer struct {
	host           host.Host
	store          ChainStore
	streamHandler  *reqresp.StreamHandler
	reqrespHandler *reqresp.Handler
	logger         *slog.Logger

	mu         sync.RWMutex
	peerStatus map[peer.ID]*reqresp.Status
	state      SyncState

	pendingParents   map[types.Root]struct{}
	pendingParentsMu sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
}

// Config holds syncer configuration.
type Config struct {
	Host           host.Host
	Store          ChainStore
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
		peerStatus:     make(map[peer.ID]*reqresp.Status),
		pendingParents: make(map[types.Root]struct{}),
		state:          SyncStateIdle,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start begins the syncer background tasks.
func (s *Syncer) Start() {
	// Register connection notifier
	s.host.Network().Notify(&connectionNotifier{syncer: s, logger: s.logger})

	// Check for existing peers (e.g., bootnodes connected before syncer started)
	for _, peerID := range s.host.Network().Peers() {
		s.logger.Debug("found existing peer, initiating status exchange", "peer", peerID)
		go func(pid peer.ID) {
			ctx, cancel := context.WithTimeout(s.ctx, reqrespTimeout)
			defer cancel()
			if err := s.InitiateStatusExchange(ctx, pid); err != nil {
				s.logger.Warn("status exchange with existing peer failed",
					"peer", pid,
					"error", err,
				)
			}
		}(peerID)
	}

	s.logger.Info("syncer started")
}

// Stop shuts down the syncer.
func (s *Syncer) Stop() {
	s.cancel()
	s.logger.Info("syncer stopped")
}

// InitiateStatusExchange sends our status and processes peer's response.
func (s *Syncer) InitiateStatusExchange(ctx context.Context, peerID peer.ID) error {
	ourStatus := s.reqrespHandler.GetStatus()

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
func (s *Syncer) processPeerStatus(peerID peer.ID, peerStatus *reqresp.Status) error {
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
	s.peerStatus[peerID] = peerStatus
	s.mu.Unlock()

	// Check if we need to sync
	ourStatus := s.reqrespHandler.GetStatus()
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
func (s *Syncer) syncFromPeer(peerID peer.ID, peerStatus *reqresp.Status) {
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

	blocks, err := s.requestBlocksWithRetry(peerID, roots)
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
				"slot", block.Message.Block.Slot,
				"error", err,
			)
		}
	}
}

// processReceivedBlock processes a block received via req/resp.
// If parent is unknown, requests parent chain.
func (s *Syncer) processReceivedBlock(block *types.SignedBlockWithAttestation, fromPeer peer.ID) error {
	innerBlock := &block.Message.Block
	blockRoot, err := innerBlock.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("hash block: %w", err)
	}

	if s.store.HasBlock(blockRoot) {
		return nil
	}

	parentRoot := innerBlock.ParentRoot
	if !s.store.HasBlock(parentRoot) {
		// Parent unknown - request parent chain
		if err := s.requestParentChain(parentRoot, fromPeer); err != nil {
			return fmt.Errorf("request parent chain: %w", err)
		}
	}

	// Process the block
	if err := s.store.ProcessBlock(innerBlock); err != nil {
		return fmt.Errorf("process block: %w", err)
	}

	s.logger.Info("synced block",
		"slot", innerBlock.Slot,
		"proposer", innerBlock.ProposerIndex,
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

	blocks, err := s.requestBlocksWithRetry(fromPeer, []types.Root{parentRoot})
	if err != nil {
		return fmt.Errorf("request parent: %w", err)
	}

	for _, block := range blocks {
		if err := s.processReceivedBlock(block, fromPeer); err != nil {
			s.logger.Warn("failed to process parent block",
				"slot", block.Message.Block.Slot,
				"error", err,
			)
		}
	}

	return nil
}

// requestBlocksWithRetry wraps RequestBlocksByRoot with exponential backoff retry.
// Retries up to maxSyncRetries (3) times with delays of 1s, 2s, 4s.
// This handles transient libp2p stream reset errors that can occur under load.
func (s *Syncer) requestBlocksWithRetry(peerID peer.ID, roots []types.Root) ([]*types.SignedBlockWithAttestation, error) {
	var lastErr error
	for attempt := 0; attempt <= maxSyncRetries; attempt++ {
		if attempt > 0 {
			delay := baseRetryDelay * time.Duration(1<<(attempt-1)) // 1s, 2s, 4s
			s.logger.Debug("retrying block request",
				"peer", peerID,
				"attempt", attempt+1,
				"delay", delay,
			)
			select {
			case <-s.ctx.Done():
				return nil, s.ctx.Err()
			case <-time.After(delay):
			}
		}

		blocks, err := s.streamHandler.RequestBlocksByRoot(s.ctx, peerID, roots)
		if err == nil {
			return blocks, nil
		}
		lastErr = err
		s.logger.Debug("block request failed",
			"peer", peerID,
			"attempt", attempt+1,
			"error", err,
		)
	}
	return nil, fmt.Errorf("after %d retries: %w", maxSyncRetries, lastErr)
}

// RemovePeer removes a peer from tracking.
func (s *Syncer) RemovePeer(peerID peer.ID) {
	s.mu.Lock()
	delete(s.peerStatus, peerID)
	s.mu.Unlock()
}

func (s *Syncer) OnBlockReceived(block *types.SignedBlockWithAttestation, fromPeer peer.ID) error {
	parentRoot := block.Message.Block.ParentRoot
	if !s.store.HasBlock(parentRoot) {
		return s.requestParentChain(parentRoot, fromPeer)
	}
	return nil
}

// connectionNotifier listens for peer connection events.
type connectionNotifier struct {
	syncer *Syncer
	logger *slog.Logger
}

// Listen implements network.Notifiee
func (n *connectionNotifier) Listen(network.Network, multiaddr.Multiaddr) {}

// ListenClose implements network.Notifiee
func (n *connectionNotifier) ListenClose(network.Network, multiaddr.Multiaddr) {}

// Connected is called when a new peer connection is established.
// The dialer sends Status first; the listener responds with its own.
func (n *connectionNotifier) Connected(net network.Network, conn network.Conn) {
	peerID := conn.RemotePeer()

	// Check if we initiated the connection (we are the dialer)
	if conn.Stat().Direction == network.DirOutbound {
		// We dialed, we send status first
		n.logger.Debug("new outbound connection, initiating status exchange", "peer", peerID)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), reqrespTimeout)
			defer cancel()
			if err := n.syncer.InitiateStatusExchange(ctx, peerID); err != nil {
				n.logger.Warn("status exchange failed", "peer", peerID, "error", err)
			}
		}()
	} else {
		n.logger.Debug("new inbound connection", "peer", peerID)
		// If we are the listener, we wait for them to send status first
		// (handled by the stream handler when they open a Status stream)
	}
}

// Disconnected is called when a peer disconnects.
func (n *connectionNotifier) Disconnected(net network.Network, conn network.Conn) {
	peerID := conn.RemotePeer()
	n.logger.Debug("peer disconnected", "peer", peerID)
	n.syncer.RemovePeer(peerID)
}

var _ network.Notifiee = (*connectionNotifier)(nil)
