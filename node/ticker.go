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

	// Register gossip handlers before syncing so blocks produced by peers
	// during initial sync are not silently dropped. leanSpec requires nodes
	// to subscribe to topics before connecting to peers.
	if err := n.registerGossipHandlers(); err != nil {
		n.log.Error("failed to register gossip handlers", "err", err)
		return err
	}
	n.log.Info("gossip handlers registered, starting initial sync")
	n.initialSync(ctx)
	n.log.Info("initial sync completed")

	var lastLogSlot uint64 = ^uint64(0)

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

		// Always execute validator duties on the clock.
		// No sync gating — matches ethlambda/leanSpec. Gossip delivers blocks,
		// the fetcher handles missing parents asynchronously.
		n.Validator.OnInterval(ctx, slot, interval)

		// Update metrics and log on slot boundary.
		if slot != lastLogSlot {
			start := time.Now()
			// Refresh status for metrics if not already current.
			status := n.FC.GetStatus()

			metrics.CurrentSlot.Set(float64(slot))
			metrics.HeadSlot.Set(float64(status.HeadSlot))
			metrics.LatestFinalizedSlot.Set(float64(status.FinalizedSlot))
			metrics.LatestJustifiedSlot.Set(float64(status.JustifiedSlot))
			peerCount := len(n.Host.P2P.Network().Peers())
			metrics.ConnectedPeers.WithLabelValues("gean").Set(float64(peerCount))

			n.log.Info("slot",
				"slot", slot,
				"head_slot", status.HeadSlot,
				"head_root", logging.LongHash(status.Head),
				"finalized_slot", status.FinalizedSlot,
				"finalized_root", logging.LongHash(status.FinalizedRoot),
				"justified_slot", status.JustifiedSlot,
				"justified_root", logging.LongHash(status.JustifiedRoot),
				"peers", peerCount,
				"elapsed", logging.TimeSince(start),
			)
			lastLogSlot = slot
		}
	}
}
