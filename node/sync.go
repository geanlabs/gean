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

// syncDeduplication tracks recently requested roots with exponential backoff.
// Implements instant dedup + exponential backoff on retry.
type syncDeduplication struct {
	mu       sync.Mutex
	roots    map[[32]byte]time.Time
	attempts map[[32]byte]int
	cleanup  time.Duration
}

const (
	initialBackoffMs = 5
	backoffMult      = 2
	maxRetries       = 10
)

func newSyncDeduplication() *syncDeduplication {
	return &syncDeduplication{
		roots:    make(map[[32]byte]time.Time),
		attempts: make(map[[32]byte]int),
		cleanup:  5 * time.Minute,
	}
}

func (s *syncDeduplication) shouldRequest(root [32]byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Clean up old entries
	for r, t := range s.roots {
		if now.Sub(t) > s.cleanup {
			delete(s.roots, r)
			delete(s.attempts, r)
		}
	}

	// Check if already requested
	if t, exists := s.roots[root]; exists {
		// Exponential backoff: wait before retry
		attempt := s.attempts[root]
		backoff := time.Duration(initialBackoffMs*powInt(backoffMult, attempt)) * time.Millisecond
		if now.Sub(t) < backoff {
			return false
		}
		// Backoff expired, increment attempt
		s.attempts[root] = attempt + 1
		if s.attempts[root] > maxRetries {
			return false
		}
	}

	s.roots[root] = now
	return true
}

func (s *syncDeduplication) recordSuccess(root [32]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.roots, root)
	delete(s.attempts, root)
}

func powInt(base, exp int) int {
	result := 1
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result
}

// peerFailureTracker prevents requesting from peers that failed recently.
// Tracks failed peers per root to avoid retrying the same peer.
type peerFailureTracker struct {
	mu      sync.Mutex
	failed  map[[32]byte]map[peer.ID]struct{}
	cleanup time.Duration
}

func newPeerFailureTracker() *peerFailureTracker {
	return &peerFailureTracker{
		failed:  make(map[[32]byte]map[peer.ID]struct{}),
		cleanup: 5 * time.Minute,
	}
}

func (p *peerFailureTracker) shouldTryPeer(root [32]byte, pid peer.ID) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if peers, ok := p.failed[root]; ok {
		if _, failed := peers[pid]; failed {
			return false
		}
	}
	return true
}

func (p *peerFailureTracker) recordFailure(root [32]byte, pid peer.ID) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.failed[root] == nil {
		p.failed[root] = make(map[peer.ID]struct{})
	}
	p.failed[root][pid] = struct{}{}
}

func (p *peerFailureTracker) recordSuccess(root [32]byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.failed, root)
}

var globalSyncDedup = newSyncDeduplication()
var globalPeerFailures = newPeerFailureTracker()

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

	// Walk backwards fetching blocks, collecting for forward processing
	var pending []*types.SignedBlockWithAttestation
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

		// Skip if already requested (instant dedup + backoff)
		if !globalSyncDedup.shouldRequest(nextRoot) {
			n.log.Debug("skipping root due to dedup/backoff",
				"peer_id", pid.String(),
				"root", logging.LongHash(nextRoot),
			)
			break
		}

		// Skip if already in pending blocks cache
		if n.PendingBlocks.Has(nextRoot) {
			n.log.Debug("skipping root already pending",
				"peer_id", pid.String(),
				"root", logging.LongHash(nextRoot),
			)
			nextRoot = [32]byte{}
			break
		}

		// Skip if peer previously failed for this root
		if !globalPeerFailures.shouldTryPeer(nextRoot, pid) {
			n.log.Debug("skipping peer that failed for this root",
				"peer_id", pid.String(),
				"root", logging.LongHash(nextRoot),
			)
			break
		}

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
			globalPeerFailures.recordFailure(nextRoot, pid)
			break
		}
		if len(blocks) == 0 {
			n.log.Debug("blocks_by_root returned empty",
				"peer_id", pid.String(),
				"requested_root", logging.LongHash(nextRoot),
			)
			globalPeerFailures.recordFailure(nextRoot, pid)
			break
		}

		// Collect the fetched block for processing
		pending = append(pending, blocks[0])
		globalSyncDedup.recordSuccess(nextRoot)
		nextRoot = blocks[0].Message.Block.ParentRoot
	}

	// If we couldn't collect any blocks, nothing to sync
	if len(pending) == 0 {
		return false
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
