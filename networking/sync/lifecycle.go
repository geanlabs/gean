// lifecycle.go contains syncer startup, shutdown, and peer tracking lifecycle methods.
package sync

import (
	"context"

	"github.com/libp2p/go-libp2p/core/peer"
)

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

// RemovePeer removes a peer from tracking.
func (s *Syncer) RemovePeer(peerID peer.ID) {
	s.mu.Lock()
	delete(s.peerStatus, peerID)
	s.mu.Unlock()
}
