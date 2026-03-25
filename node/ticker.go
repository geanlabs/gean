package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/observability/metrics"
)

// syncProgress tracks sync state for logging and metrics
type syncProgress struct {
	mu                      sync.Mutex
	consecutiveSyncAttempts int
	consecutiveSkippedSlots int
	lastSyncedSlot          uint64
	lastSyncGapMax          uint64
}

func newSyncProgress() *syncProgress {
	return &syncProgress{}
}

func (s *syncProgress) recordSyncAttempt(gap uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consecutiveSyncAttempts++
	s.consecutiveSkippedSlots = 0
	if gap > s.lastSyncGapMax {
		s.lastSyncGapMax = gap
	}
}

func (s *syncProgress) recordSkippedSlot() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consecutiveSkippedSlots++
}

func (s *syncProgress) recordSynced(slot uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSyncedSlot = slot
	s.consecutiveSkippedSlots = 0
	s.consecutiveSyncAttempts = 0
}

func (s *syncProgress) getStats() (int, int, uint64, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.consecutiveSyncAttempts, s.consecutiveSkippedSlots, s.lastSyncGapMax, s.lastSyncedSlot
}

var globalSyncProgress = newSyncProgress()

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

	var lastSyncCheckSlot uint64 = ^uint64(0)
	var lastLogSlot uint64 = ^uint64(0)
	var lastFinalizedSlot uint64
	behindPeers := false
	maxPeerHeadSlot := uint64(0)

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

		// Re-evaluate sync gating once per slot using peer head status.
		if slot != lastSyncCheckSlot {
			behindPeers, maxPeerHeadSlot = n.isBehindPeers(ctx, status)
			if behindPeers {
				// Proactively sync with peers when behind.
				for _, pid := range n.Host.P2P.Network().Peers() {
					if n.syncWithPeer(ctx, pid) {
						break // re-evaluate on next slot
					}
				}
				// Re-check after sync attempt.
				status = n.FC.GetStatus()
				behindPeers = status.HeadSlot < maxPeerHeadSlot

				if behindPeers {
					globalSyncProgress.recordSkippedSlot()
					_, skipped, _, _ := globalSyncProgress.getStats()
					n.log.Warn(
						"skipping validator duties while behind peers",
						"slot", slot,
						"head_slot", status.HeadSlot,
						"finalized_slot", status.FinalizedSlot,
						"max_peer_head_slot", maxPeerHeadSlot,
						"gap_slots", maxPeerHeadSlot-status.HeadSlot,
						"consecutive_skipped_slots", skipped,
					)
				}
			}
			lastSyncCheckSlot = slot
		}

		// Execute validator duties unless we are behind peers' head.
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
			metrics.ConnectedPeers.WithLabelValues("gean").Set(float64(peerCount))

			n.log.Info("slot",
				"slot", slot,
				"head_slot", status.HeadSlot,
				"head_root", logging.LongHash(status.Head),
				"finalized_slot", status.FinalizedSlot,
				"finalized_root", logging.LongHash(status.FinalizedRoot),
				"justified_slot", status.JustifiedSlot,
				"justified_root", logging.LongHash(status.JustifiedRoot),
				"behind_peers", behindPeers,
				"max_peer_head", maxPeerHeadSlot,
				"peers", peerCount,
				"elapsed", logging.TimeSince(start),
			)
			lastLogSlot = slot
		}

		// Prune pending blocks when finalization advances.
		if status.FinalizedSlot > lastFinalizedSlot {
			if pruned := n.PendingBlocks.PruneFinalized(status.FinalizedSlot); pruned > 0 {
				n.log.Info("pruned finalized pending blocks",
					"pruned", pruned,
					"finalized_slot", status.FinalizedSlot,
				)
			}
			lastFinalizedSlot = status.FinalizedSlot
		}
	}
}
