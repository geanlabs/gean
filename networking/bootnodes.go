// bootnodes.go contains bootnode reconnect retry behavior.
package networking

import (
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// retryBootnodes periodically retries connecting to failed bootnodes.
func (s *Service) retryBootnodes() {
	defer s.wg.Done()

	ticker := time.NewTicker(bootnodeRetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			var remaining []peer.AddrInfo
			for _, pi := range s.failedBootnodes {
				if err := s.host.Connect(s.ctx, pi); err != nil {
					s.logger.Debug("bootnode reconnect failed", "peer", pi.ID, "error", err)
					remaining = append(remaining, pi)
				} else {
					s.logger.Info("reconnected to bootnode", "peer", pi.ID)
				}
			}
			s.failedBootnodes = remaining
			if len(s.failedBootnodes) == 0 {
				s.logger.Debug("all bootnodes connected, stopping retry")
				return
			}
		}
	}
}
