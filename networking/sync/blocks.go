// blocks.go contains block ingestion, parent recovery, and req/resp retry logic.
package sync

import (
	"fmt"
	"time"

	"github.com/devylongs/gean/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

func (s *Syncer) OnBlockReceived(block *types.SignedBlockWithAttestation, fromPeer peer.ID) error {
	parentRoot := block.Message.Block.ParentRoot
	if !s.store.HasBlock(parentRoot) {
		return s.requestParentChain(parentRoot, fromPeer)
	}
	return nil
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
