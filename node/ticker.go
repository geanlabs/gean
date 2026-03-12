package node

import (
	"context"
	"fmt"
	"time"

	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/observability/metrics"
)

// Run starts the main event loop.
func (n *Node) Run(ctx context.Context) error {
	n.log.Info("node started",
		"validators", fmt.Sprintf("%v", n.Validator.Indices),
		"peers", len(n.Host.P2P.Network().Peers()),
	)

	// Attempt initial sync with connected peers.
	n.initialSync(ctx)

	var lastSyncCheckSlot uint64 = ^uint64(0)
	var lastLogSlot uint64 = ^uint64(0)
	behindPeers := false
	maxPeerFinalizedSlot := uint64(0)

	for {
		wait := n.Clock.DurationUntilNextInterval()
		timer := time.NewTimer(wait)

		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			n.log.Info("node shutting down")
			if n.API != nil {
				n.API.Stop()
			}
			if err := n.Host.Close(); err != nil {
				n.log.Warn("host close error", "err", err)
			}
			return nil
		case <-timer.C:
		}

		if n.Clock.IsBeforeGenesis() {
			continue
		}
		slot := n.Clock.CurrentSlot()
		interval := n.Clock.CurrentInterval()
		hasProposal := interval == 0 && n.Validator.HasProposal(slot)

		// Advance fork choice time.
		n.FC.AdvanceTimeMillis(n.Clock.CurrentTime(), hasProposal)

		status := n.FC.GetStatus()

		// Re-evaluate sync gating once per slot using peer finalization status.
		if slot != lastSyncCheckSlot {
			behindPeers, maxPeerFinalizedSlot = n.isBehindPeerFinalization(ctx, status)
			if behindPeers {
				for _, pid := range n.Host.P2P.Network().Peers() {
					if n.syncWithPeer(ctx, pid) {
						status = n.FC.GetStatus()
					}
				}
				behindPeers, maxPeerFinalizedSlot = n.isBehindPeerFinalization(ctx, status)
				if behindPeers {
					n.log.Warn(
						"skipping validator duties while behind peer finalization",
						"slot", slot,
						"head_slot", status.HeadSlot,
						"finalized_slot", status.FinalizedSlot,
						"max_peer_finalized_slot", maxPeerFinalizedSlot,
					)
				}
			}
			lastSyncCheckSlot = slot
		}

		// Execute validator duties unless we are behind peers' finalized checkpoint.
		if !behindPeers {
			n.Validator.OnInterval(ctx, slot, interval)
		}

		// Update metrics and log on slot boundary.
		if slot != lastLogSlot {
			start := time.Now()
			// Refresh status for metrics if not already current.
			status = n.FC.GetStatus()

			metrics.CurrentSlot.Set(float64(slot))
			metrics.HeadSlot.Set(float64(status.HeadSlot))
			metrics.LatestFinalizedSlot.Set(float64(status.FinalizedSlot))
			metrics.LatestJustifiedSlot.Set(float64(status.JustifiedSlot))
			peerCount := len(n.Host.P2P.Network().Peers())
			metrics.ConnectedPeers.Set(float64(peerCount))

			n.log.Info("slot",
				"slot", slot,
				"head", status.HeadSlot,
				"finalized", status.FinalizedSlot,
				"justified", status.JustifiedSlot,
				"behind_peer_finalization", behindPeers,
				"max_peer_finalized", maxPeerFinalizedSlot,
				"peers", peerCount,
				"elapsed", logging.TimeSince(start),
			)
			lastLogSlot = slot
		}
	}
}
