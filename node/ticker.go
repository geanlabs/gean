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

	ticker := n.Clock.SlotTicker()
	var lastSlot uint64

	for {
		select {
		case <-ctx.Done():
			n.log.Info("node shutting down")
			if err := n.Host.Close(); err != nil {
				n.log.Warn("host close error", "err", err)
			}
			return nil
		case <-ticker:
			if n.Clock.IsBeforeGenesis() {
				continue
			}
			slot := n.Clock.CurrentSlot()
			interval := n.Clock.CurrentInterval()
			hasProposal := interval == 0 && n.Validator.HasProposal(slot)

			// Advance fork choice time.
			n.FC.AdvanceTime(n.Clock.CurrentTime(), hasProposal)

			// Execute validator duties.
			n.Validator.OnInterval(ctx, slot, interval)

			// Update metrics and log on slot boundary.
			if slot != lastSlot {
				start := time.Now()
				metrics.CurrentSlot.Set(float64(slot))
				headSlot := uint64(0)
				if headBlock, ok := n.FC.Storage.GetBlock(n.FC.Head); ok {
					headSlot = headBlock.Slot
					metrics.HeadSlot.Set(float64(headBlock.Slot))
				}
				metrics.LatestFinalizedSlot.Set(float64(n.FC.LatestFinalized.Slot))
				metrics.LatestJustifiedSlot.Set(float64(n.FC.LatestJustified.Slot))
				peerCount := len(n.Host.P2P.Network().Peers())
				metrics.ConnectedPeers.Set(float64(peerCount))

				// Periodic sync: if head is behind, try catching up.
				if slot > headSlot+2 {
					for _, pid := range n.Host.P2P.Network().Peers() {
						if n.syncWithPeer(ctx, pid) {
							break
						}
					}
				}

				n.log.Info("slot",
					"slot", slot,
					"head", headSlot,
					"finalized", n.FC.LatestFinalized.Slot,
					"justified", n.FC.LatestJustified.Slot,
					"peers", peerCount,
					"elapsed", logging.TimeSince(start),
				)
				lastSlot = slot
			}
		}
	}
}
