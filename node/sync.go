package node

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/observability/metrics"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/network/reqresp"
	"github.com/geanlabs/gean/types"
)

// syncDeduplication tracks recently requested roots to prevent duplicate requests.
// Per leanSpec: nodes should not request the same blocks multiple times.
type syncDeduplication struct {
	mu      sync.Mutex
	roots   map[[32]byte]time.Time
	cleanup time.Duration
}

func newSyncDeduplication() *syncDeduplication {
	return &syncDeduplication{
		roots:   make(map[[32]byte]time.Time),
		cleanup: 30 * time.Second,
	}
}

func (s *syncDeduplication) shouldRequest(root [32]byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clean up old entries
	now := time.Now()
	for r, t := range s.roots {
		if now.Sub(t) > s.cleanup {
			delete(s.roots, r)
		}
	}

	// Check if already requested recently
	if _, exists := s.roots[root]; exists {
		return false
	}

	s.roots[root] = now
	return true
}

// globalSyncDedup prevents duplicate block requests across all sync operations
var globalSyncDedup = newSyncDeduplication()

func isMissingParentStateErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "parent state not found")
}

// syncWithPeer exchanges status and fetches missing blocks from a single peer.
// It walks backwards from the peer's head to find blocks we're missing, then
// processes them in forward order.
func (n *Node) syncWithPeer(ctx context.Context, pid peer.ID) bool {
	status := n.FC.GetStatus()
	ourStatus := reqresp.Status{
		Finalized: &types.Checkpoint{Root: status.FinalizedRoot, Slot: status.FinalizedSlot},
		Head:      &types.Checkpoint{Root: status.Head, Slot: status.HeadSlot},
	}

	peerStatus, err := reqresp.RequestStatus(ctx, n.Host.P2P, pid, ourStatus)
	metrics.SyncStatusExchangesTotal.WithLabelValues("success").Inc()
	if err != nil {
		n.log.Debug("status exchange failed",
			"peer_id", pid.String(),
			"local_head_slot", status.HeadSlot,
			"err", err,
		)
		metrics.SyncStatusExchangesTotal.WithLabelValues("error").Inc()
		return false
	}
	n.log.Info("status exchanged",
		"peer_id", pid.String(),
		"local_head_slot", status.HeadSlot,
		"local_head_root", logging.LongHash(status.Head),
		"local_finalized_slot", status.FinalizedSlot,
		"local_finalized_root", logging.LongHash(status.FinalizedRoot),
		"peer_head_slot", peerStatus.Head.Slot,
		"peer_head_root", logging.LongHash(peerStatus.Head.Root),
		"peer_finalized_slot", peerStatus.Finalized.Slot,
		"peer_finalized_root", logging.LongHash(peerStatus.Finalized.Root),
	)

	// Skip sync only if peer is strictly behind us, or at the exact same position.
	// If peer is at the same slot but with a different head root, we should still
	// sync to ensure we have their chain (potential re-org or fork).
	if peerStatus.Head.Slot < status.HeadSlot {
		n.log.Debug("peer behind us, skipping",
			"peer_id", pid.String(),
			"peer_head_slot", peerStatus.Head.Slot,
			"our_head_slot", status.HeadSlot,
		)
		return false
	}
	if peerStatus.Head.Slot == status.HeadSlot && peerStatus.Head.Root == status.Head {
		n.log.Debug("peer at same head, skipping",
			"peer_id", pid.String(),
			"head_slot", status.HeadSlot,
			"head_root", logging.LongHash(status.Head),
		)
		return false
	}

	// Phase 1: Walk backwards collecting roots we need (for batched request)
	// Per leanSpec: nodes should batch block requests when syncing
	var neededRoots [][32]byte
	nextRoot := peerStatus.Head.Root
	backlog := uint64(1)
	if peerStatus.Head.Slot > status.HeadSlot {
		backlog = peerStatus.Head.Slot - status.HeadSlot
	}
	maxSyncDepth := int(backlog + 16)
	const maxSyncDepthCap = 32768
	if maxSyncDepth > maxSyncDepthCap {
		maxSyncDepth = maxSyncDepthCap
	}

	n.log.Info("starting sync walk",
		"peer_id", pid.String(),
		"peer_head_slot", peerStatus.Head.Slot,
		"peer_head_root", logging.LongHash(peerStatus.Head.Root),
		"our_head_slot", status.HeadSlot,
		"gap_slots", backlog,
		"max_depth", maxSyncDepth,
	)
	metrics.SyncGapSlots.Set(float64(backlog))

	for i := 0; i < maxSyncDepth; i++ {
		if n.FC.HasState(nextRoot) {
			break
		}

		// Skip if already requested (deduplication per leanSpec)
		if !globalSyncDedup.shouldRequest(nextRoot) {
			n.log.Debug("skipping already-requested root",
				"peer_id", pid.String(),
				"root", logging.LongHash(nextRoot),
			)
			break
		}

		neededRoots = append(neededRoots, nextRoot)
		n.log.Info("blocks_by_root requesting for parent chain",
			"peer_id", pid.String(),
			"root", logging.LongHash(nextRoot),
			"walk_depth", i+1,
		)
		reqStart := time.Now()
		blocks, err := reqresp.RequestBlocksByRoot(ctx, n.Host.P2P, pid, [][32]byte{nextRoot})
		metrics.BlocksByRootRequestsTotal.WithLabelValues("outbound").Inc()
		metrics.BlocksByRootResponseDuration.Observe(time.Since(reqStart).Seconds())

		if err != nil {
			n.log.Warn("blocks_by_root failed during sync walk",
				"peer_id", pid.String(),
				"requested_root", logging.LongHash(nextRoot),
				"err", err,
			)
			break
		}
		if len(blocks) == 0 {
			n.log.Debug("blocks_by_root returned empty",
				"peer_id", pid.String(),
				"requested_root", logging.LongHash(nextRoot),
			)
			break
		}
		nextRoot = blocks[0].Message.Block.ParentRoot
	}

	// If we couldn't collect any roots, nothing to sync
	if len(neededRoots) == 0 {
		return false
	}

	// Phase 2: Request all collected roots in a batch
	// Per leanSpec: batch requests reduce network overhead
	n.log.Info("blocks_by_root batch request",
		"peer_id", pid.String(),
		"roots_count", len(neededRoots),
		"first_slot", func() uint64 {
			if len(neededRoots) > 0 {
				return peerStatus.Head.Slot - uint64(len(neededRoots)-1)
			}
			return 0
		}(),
		"last_slot", peerStatus.Head.Slot,
	)

	startTime := time.Now()
	blocks, err := reqresp.RequestBlocksByRoot(ctx, n.Host.P2P, pid, neededRoots)
	metrics.BlocksByRootRequestsTotal.WithLabelValues("outbound").Inc()
	duration := time.Since(startTime)
	metrics.BlocksByRootResponseDuration.Observe(duration.Seconds())

	if err != nil {
		n.log.Warn("blocks_by_root batch request failed",
			"peer_id", pid.String(),
			"roots_count", len(neededRoots),
			"err", err,
		)
		return false
	}

	n.log.Info("blocks_by_root batch response received",
		"peer_id", pid.String(),
		"requested", len(neededRoots),
		"received", len(blocks),
		"duration_ms", duration.Milliseconds(),
	)

	// Build pending list from batched response
	var pending []*types.SignedBlockWithAttestation
	for _, sb := range blocks {
		pending = append(pending, sb)
	}

	if !n.FC.HasState(nextRoot) {
		n.log.Debug("sync walk did not reach known ancestor with state",
			"peer_id", pid.String(),
			"ancestor_root", logging.LongHash(nextRoot),
			"fetched", len(pending),
			"max_depth", maxSyncDepth,
		)
		return false
	}

	// Process in forward order (oldest first).
	synced := 0
	total := len(pending)
	for i := len(pending) - 1; i >= 0; i-- {
		sb := pending[i]
		blockRoot, _ := sb.Message.Block.HashTreeRoot()
		if err := n.FC.ProcessBlock(sb); err != nil {
			n.log.Warn("sync block rejected",
				"slot", sb.Message.Block.Slot,
				"block_root", logging.LongHash(blockRoot),
				"peer_id", pid.String(),
				"err", err,
			)
		} else {
			synced++
			metrics.SyncBlocksDownloadedTotal.Inc()
			if synced == 1 || synced == total || synced%10 == 0 {
				n.log.Info("synced block",
					"slot", sb.Message.Block.Slot,
					"block_root", logging.LongHash(blockRoot),
					"parent_root", logging.LongHash(sb.Message.Block.ParentRoot),
					"proposer", sb.Message.Block.ProposerIndex,
					"peer_id", pid.String(),
					"progress", fmt.Sprintf("%d/%d", synced, total),
				)
			}
		}
	}

	finalStatus := n.FC.GetStatus()
	n.log.Info("sync with peer completed",
		"peer_id", pid.String(),
		"blocks_synced", synced,
		"total_requested", total,
		"new_head_slot", finalStatus.HeadSlot,
		"new_head_root", logging.LongHash(finalStatus.Head),
		"justified_slot", finalStatus.JustifiedSlot,
		"finalized_slot", finalStatus.FinalizedSlot,
	)

	return synced > 0
}

// recoverMissingParentSync attempts to fill a missing parent chain by syncing with
// connected peers, then checks whether the requested parent state became available.
func (n *Node) recoverMissingParentSync(ctx context.Context, parentRoot [32]byte) bool {
	if n.FC.HasState(parentRoot) {
		return true
	}

	for _, pid := range n.Host.P2P.Network().Peers() {
		n.syncWithPeer(ctx, pid)
		if n.FC.HasState(parentRoot) {
			return true
		}
	}
	return false
}

// initialSync exchanges status with connected peers and requests any blocks
// we're missing. This allows a node that restarts mid-devnet to catch up.
func (n *Node) initialSync(ctx context.Context) {
	peers := n.Host.P2P.Network().Peers()
	n.log.Info("initial sync starting",
		"peer_count", len(peers),
		"head_slot", n.FC.GetStatus().HeadSlot,
	)

	syncStart := time.Now()
	successCount := 0
	for _, pid := range peers {
		if n.syncWithPeer(ctx, pid) {
			successCount++
		}
	}

	status := n.FC.GetStatus()
	elapsed := time.Since(syncStart)
	n.log.Info("initial sync completed",
		"head_slot", status.HeadSlot,
		"head_root", logging.LongHash(status.Head),
		"justified_slot", status.JustifiedSlot,
		"justified_root", logging.LongHash(status.JustifiedRoot),
		"finalized_slot", status.FinalizedSlot,
		"finalized_root", logging.LongHash(status.FinalizedRoot),
		"peers_synced", successCount,
		"total_peers", len(peers),
		"elapsed_ms", elapsed.Milliseconds(),
	)
}

// isBehindPeers reports whether our head is behind the highest head slot
// advertised by connected peers. This fires even when finalization is stalled,
// ensuring we sync from peers who have blocks we haven't yet received via gossip.
func (n *Node) isBehindPeers(ctx context.Context, status forkchoice.ChainStatus) (bool, uint64) {
	maxPeerHeadSlot := status.HeadSlot
	peers := n.Host.P2P.Network().Peers()
	if len(peers) == 0 {
		return false, maxPeerHeadSlot
	}

	ourStatus := reqresp.Status{
		Finalized: &types.Checkpoint{Root: status.FinalizedRoot, Slot: status.FinalizedSlot},
		Head:      &types.Checkpoint{Root: status.Head, Slot: status.HeadSlot},
	}

	for _, pid := range peers {
		peerCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
		peerStatus, err := reqresp.RequestStatus(peerCtx, n.Host.P2P, pid, ourStatus)
		cancel()
		if err != nil || peerStatus.Head == nil {
			continue
		}
		if peerStatus.Head.Slot > maxPeerHeadSlot {
			maxPeerHeadSlot = peerStatus.Head.Slot
		}
	}

	behind := status.HeadSlot < maxPeerHeadSlot
	return behind, maxPeerHeadSlot
}
