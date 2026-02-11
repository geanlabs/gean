// status.go contains status handshake handling and sync trigger orchestration.
package sync

import (
	"context"
	"fmt"
	"time"

	"github.com/devylongs/gean/networking/reqresp"
	"github.com/devylongs/gean/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

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
