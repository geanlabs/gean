package node

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/network/reqresp"
	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

func isMissingParentStateErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "parent state not found")
}

// syncWithPeer exchanges status and fetches missing blocks from a single peer.
// It walks backwards from the peer's head collecting roots we need, then
// requests them in batches (up to maxBlocksPerRequest per RPC call) and
// processes them in forward order.
//
// The backward walk is capped at maxBackfillDepth (512) to prevent resource
// exhaustion from deep chains, matching leanSpec MAX_BACKFILL_DEPTH.
func (n *Node) syncWithPeer(ctx context.Context, pid peer.ID) bool {
	if !n.canSyncWithPeer(pid) {
		return false
	}

	status := n.FC.GetStatus()
	ourStatus := reqresp.Status{
		Finalized: &types.Checkpoint{Root: status.FinalizedRoot, Slot: status.FinalizedSlot},
		Head:      &types.Checkpoint{Root: status.Head, Slot: status.HeadSlot},
	}

	peerStatus, err := reqresp.RequestStatus(ctx, n.Host.P2P, pid, ourStatus)
	if err != nil {
		n.log.Debug("status exchange failed", "peer_id", pid.String(), "err", err)
		n.recordSyncFailure(pid)
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
	if peerStatus.Head.Slot < status.HeadSlot {
		return false
	}
	if peerStatus.Head.Slot == status.HeadSlot && peerStatus.Head.Root == status.Head {
		return false
	}

	// Phase 1: Walk backwards collecting roots we need to fetch.
	// Stop when we find a root we have state for, or hit the depth limit.
	var rootsToFetch [][32]byte
	nextRoot := peerStatus.Head.Root

	for i := 0; i < maxBackfillDepth; i++ {
		if n.FC.HasState(nextRoot) {
			break
		}

		// Skip roots already being fetched by another sync path.
		if n.isRootPending(nextRoot) {
			n.log.Debug("skipping already-pending root", "root", logging.LongHash(nextRoot))
			break
		}

		// Request this single root to discover its parent for the walk.
		// We need the block to learn its ParentRoot for the next step.
		blocks, err := reqresp.RequestBlocksByRoot(ctx, n.Host.P2P, pid, [][32]byte{nextRoot})
		if err != nil || len(blocks) == 0 {
			n.log.Debug("blocks_by_root failed during sync walk",
				"peer_id", pid.String(),
				"requested_root", logging.LongHash(nextRoot),
				"err", err,
			)
			n.recordSyncFailure(pid)
			break
		}

		sb := blocks[0]
		rootsToFetch = append(rootsToFetch, nextRoot)
		nextRoot = sb.Message.Block.ParentRoot
	}

	if len(rootsToFetch) == 0 {
		return false
	}

	// Check if we reached a known ancestor.
	if !n.FC.HasState(nextRoot) {
		n.log.Debug("sync walk did not reach known ancestor with state",
			"peer_id", pid.String(),
			"ancestor_root", logging.LongHash(nextRoot),
			"collected", len(rootsToFetch),
			"max_depth", maxBackfillDepth,
		)
		return false
	}

	// Mark all roots as pending to prevent duplicate fetches.
	n.markRootsPending(rootsToFetch)
	defer n.clearPendingRoots(rootsToFetch)

	// Phase 2: Fetch blocks in batches of maxBlocksPerRequest (10).
	// Roots are in newest-first order; we reverse each batch for forward processing.
	var allBlocks []*types.SignedBlockWithAttestation

	for batchStart := 0; batchStart < len(rootsToFetch); batchStart += maxBlocksPerRequest {
		batchEnd := batchStart + maxBlocksPerRequest
		if batchEnd > len(rootsToFetch) {
			batchEnd = len(rootsToFetch)
		}
		batch := rootsToFetch[batchStart:batchEnd]

		blocks, err := reqresp.RequestBlocksByRoot(ctx, n.Host.P2P, pid, batch)
		if err != nil {
			n.log.Warn("batch blocks_by_root failed",
				"peer_id", pid.String(),
				"batch_size", len(batch),
				"err", err,
			)
			n.recordSyncFailure(pid)
			break
		}
		allBlocks = append(allBlocks, blocks...)
	}

	if len(allBlocks) == 0 {
		return false
	}

	// Phase 3: Process in forward order (oldest first).
	synced := 0
	total := len(allBlocks)
	for i := len(allBlocks) - 1; i >= 0; i-- {
		sb := allBlocks[i]
		blockRoot, _ := sb.Message.Block.HashTreeRoot()
		if err := n.FC.ProcessBlock(sb); err != nil {
			n.log.Debug("sync block rejected",
				"slot", sb.Message.Block.Slot,
				"block_root", logging.LongHash(blockRoot),
				"err", err,
			)
		} else {
			synced++
			n.log.Info("synced block",
				"slot", sb.Message.Block.Slot,
				"block_root", logging.LongHash(blockRoot),
				"peer_id", pid.String(),
				"progress", fmt.Sprintf("%d/%d", synced, total),
			)
		}
	}

	if synced > 0 {
		n.recordSyncSuccess(pid)
	}
	return synced > 0
}

// recoverMissingParentSync attempts to fill a missing parent chain by syncing
// with connected peers. Rate-limited to prevent excessive request flooding
// when multiple gossip blocks arrive with missing parents in quick succession.
func (n *Node) recoverMissingParentSync(ctx context.Context, parentRoot [32]byte) bool {
	if n.FC.HasState(parentRoot) {
		return true
	}

	// Rate-limit: skip if a recovery attempt happened recently.
	n.recoveryMu.Lock()
	if time.Since(n.lastRecoveryTime) < recoveryCooldown {
		n.recoveryMu.Unlock()
		return false
	}
	n.lastRecoveryTime = time.Now()
	n.recoveryMu.Unlock()

	// Try peers until one succeeds.
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
	n.log.Info("initial sync starting", "peer_count", len(peers))
	for _, pid := range peers {
		n.syncWithPeer(ctx, pid)
	}
	status := n.FC.GetStatus()
	n.log.Info("initial sync completed",
		"head_slot", status.HeadSlot,
		"head_root", logging.LongHash(status.Head),
		"justified_slot", status.JustifiedSlot,
		"justified_root", logging.LongHash(status.JustifiedRoot),
		"finalized_slot", status.FinalizedSlot,
		"finalized_root", logging.LongHash(status.FinalizedRoot),
	)
}

// isBehindPeers reports whether our head is behind the highest head slot
// advertised by connected peers.
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

// --- Pending roots deduplication ---

func (n *Node) isRootPending(root [32]byte) bool {
	n.pendingRootsMu.Lock()
	defer n.pendingRootsMu.Unlock()
	if n.pendingRoots == nil {
		return false
	}
	_, ok := n.pendingRoots[root]
	return ok
}

func (n *Node) markRootsPending(roots [][32]byte) {
	n.pendingRootsMu.Lock()
	defer n.pendingRootsMu.Unlock()
	if n.pendingRoots == nil {
		n.pendingRoots = make(map[[32]byte]struct{})
	}
	for _, root := range roots {
		n.pendingRoots[root] = struct{}{}
	}
}

func (n *Node) clearPendingRoots(roots [][32]byte) {
	n.pendingRootsMu.Lock()
	defer n.pendingRootsMu.Unlock()
	for _, root := range roots {
		delete(n.pendingRoots, root)
	}
}

// --- Per-peer exponential backoff ---

// canSyncWithPeer checks if enough time has passed since the last failure
// for this peer, using exponential backoff.
func (n *Node) canSyncWithPeer(pid peer.ID) bool {
	n.peerBackoffMu.Lock()
	defer n.peerBackoffMu.Unlock()
	if n.peerBackoff == nil {
		return true
	}
	state, ok := n.peerBackoff[pid]
	if !ok {
		return true
	}
	if state.failures >= maxSyncRetries {
		// Reset after max retries so peer gets another chance eventually.
		delete(n.peerBackoff, pid)
		return true
	}
	backoff := initialBackoff
	for i := 1; i < state.failures; i++ {
		backoff *= backoffMultiplier
	}
	return time.Since(state.lastTried) >= backoff
}

func (n *Node) recordSyncFailure(pid peer.ID) {
	n.peerBackoffMu.Lock()
	defer n.peerBackoffMu.Unlock()
	if n.peerBackoff == nil {
		n.peerBackoff = make(map[peer.ID]*peerSyncState)
	}
	state, ok := n.peerBackoff[pid]
	if !ok {
		state = &peerSyncState{}
		n.peerBackoff[pid] = state
	}
	state.failures++
	state.lastTried = time.Now()
}

func (n *Node) recordSyncSuccess(pid peer.ID) {
	n.peerBackoffMu.Lock()
	defer n.peerBackoffMu.Unlock()
	if n.peerBackoff != nil {
		delete(n.peerBackoff, pid)
	}
}
