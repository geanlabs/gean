package node

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/network/reqresp"
	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Sync constants aligned with leanSpec where applicable.
const (
	maxBackfillDepth     = 512              // leanSpec MAX_BACKFILL_DEPTH (was 32768)
	maxConcurrentPerPeer = 2                // leanSpec MAX_CONCURRENT_REQUESTS
	maxRecoveryPeers     = 2                // bounded peer subset for parent recovery
	maxBackfillsPerTick  = 3                // implementation default, tunable
	requestTimeout       = 8 * time.Second  // per-request context timeout
	recoveryCooldown     = 2 * time.Second  // cooldown after failed parent recovery
	inflightStaleAge     = 30 * time.Second // cleanup threshold for abandoned inflight entries
)

// ---------------------------------------------------------------------------
// inflightRoots — deduplicates concurrent outbound requests for the same root.
// ---------------------------------------------------------------------------

type inflightRoots struct {
	mu    sync.Mutex
	roots map[[32]byte]time.Time
}

func newInflightRoots() *inflightRoots {
	return &inflightRoots{roots: make(map[[32]byte]time.Time)}
}

// tryAcquire marks root as in-flight. Returns false if already in-flight.
func (r *inflightRoots) tryAcquire(root [32]byte) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.roots[root]; exists {
		return false
	}
	r.roots[root] = time.Now()
	return true
}

// release removes root from in-flight tracking.
func (r *inflightRoots) release(root [32]byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.roots, root)
}

// releaseStale removes entries older than maxAge to prevent leaks from
// abandoned requests (e.g. context cancelled without cleanup).
func (r *inflightRoots) releaseStale(maxAge time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for root, ts := range r.roots {
		if ts.Before(cutoff) {
			delete(r.roots, root)
		}
	}
}

// ---------------------------------------------------------------------------
// peerLimiter — caps concurrent sync sessions per peer.
// ---------------------------------------------------------------------------

type peerLimiter struct {
	mu       sync.Mutex
	inflight map[peer.ID]int
}

func newPeerLimiter() *peerLimiter {
	return &peerLimiter{inflight: make(map[peer.ID]int)}
}

// acquire increments the in-flight count for pid. Returns false if the peer
// is already at maxConcurrentPerPeer sessions.
func (l *peerLimiter) acquire(pid peer.ID) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[pid] >= maxConcurrentPerPeer {
		return false
	}
	l.inflight[pid]++
	return true
}

// release decrements the in-flight count for pid.
func (l *peerLimiter) release(pid peer.ID) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[pid] > 0 {
		l.inflight[pid]--
	}
	if l.inflight[pid] == 0 {
		delete(l.inflight, pid)
	}
}

// ---------------------------------------------------------------------------
// recoveryCoordinator — deduplicates missing-parent recovery workflows and
// applies cooldown after failed attempts.
// ---------------------------------------------------------------------------

type recoveryCoordinator struct {
	mu        sync.Mutex
	active    map[[32]byte]time.Time // parentRoot → recovery start
	cooldowns map[[32]byte]time.Time // parentRoot → cooldown expiry
}

func newRecoveryCoordinator() *recoveryCoordinator {
	return &recoveryCoordinator{
		active:    make(map[[32]byte]time.Time),
		cooldowns: make(map[[32]byte]time.Time),
	}
}

// tryStartRecovery returns true if no recovery is active for parentRoot and
// any previous cooldown has expired. Marks recovery as active on success.
func (c *recoveryCoordinator) tryStartRecovery(parentRoot [32]byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, active := c.active[parentRoot]; active {
		return false
	}
	if expiry, ok := c.cooldowns[parentRoot]; ok && time.Now().Before(expiry) {
		return false
	}
	delete(c.cooldowns, parentRoot)
	c.active[parentRoot] = time.Now()
	return true
}

// finishRecovery marks recovery as no longer active.
func (c *recoveryCoordinator) finishRecovery(parentRoot [32]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.active, parentRoot)
}

// setCooldown sets a cooldown period during which new recovery for parentRoot
// will be rejected.
func (c *recoveryCoordinator) setCooldown(parentRoot [32]byte, d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cooldowns[parentRoot] = time.Now().Add(d)
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func isMissingParentStateErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "parent state not found")
}

// selectRandomPeers returns up to n randomly-selected peers from the list.
func selectRandomPeers(peers []peer.ID, n int) []peer.ID {
	if len(peers) <= n {
		return peers
	}
	shuffled := make([]peer.ID, len(peers))
	copy(shuffled, peers)
	rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	return shuffled[:n]
}

// ---------------------------------------------------------------------------
// fetchParentChain — bounded, parent-targeted backfill replacing the old
// recoverMissingParentSync which fanned out to all peers.
// ---------------------------------------------------------------------------

// fetchParentChain fetches the ancestor chain starting from parentRoot until
// a locally-known state is found or maxBackfillDepth is reached. It tries at
// most maxRecoveryPeers and respects inflight/peer-limiter coordination.
// Returns true if the parent state became available.
func (n *Node) fetchParentChain(ctx context.Context, parentRoot [32]byte) bool {
	if n.FC.HasState(parentRoot) {
		return true
	}

	if !n.recoveryCoord.tryStartRecovery(parentRoot) {
		n.log.Debug("backfill skipped: recovery already active or in cooldown",
			"parent_root", logging.LongHash(parentRoot),
		)
		return false
	}
	defer n.recoveryCoord.finishRecovery(parentRoot)

	peers := selectRandomPeers(n.Host.P2P.Network().Peers(), maxRecoveryPeers)
	if len(peers) == 0 {
		n.log.Debug("backfill skipped: no peers available",
			"parent_root", logging.LongHash(parentRoot),
		)
		return false
	}

	for _, pid := range peers {
		if !n.peerLimiter.acquire(pid) {
			n.log.Debug("backfill skipped peer: at session limit",
				"parent_root", logging.LongHash(parentRoot),
				"peer_id", pid.String(),
			)
			continue
		}

		success := n.backfillFromPeer(ctx, pid, parentRoot)
		n.peerLimiter.release(pid)

		if success {
			return true
		}
	}

	// All attempted peers failed — apply cooldown before retrying.
	n.recoveryCoord.setCooldown(parentRoot, recoveryCooldown)
	n.log.Info("backfill failed for all peers, applying cooldown",
		"parent_root", logging.LongHash(parentRoot),
		"cooldown", recoveryCooldown,
		"peers_tried", len(peers),
	)
	return false
}

// backfillFromPeer walks backward from targetRoot fetching one block at a
// time until a known state is reached or the depth limit is hit.
func (n *Node) backfillFromPeer(ctx context.Context, pid peer.ID, targetRoot [32]byte) bool {
	var pending []*types.SignedBlockWithAttestation
	nextRoot := targetRoot

	n.log.Info("backfill started",
		"parent_root", logging.LongHash(targetRoot),
		"peer_id", pid.String(),
		"max_depth", maxBackfillDepth,
	)

	for depth := 0; depth < maxBackfillDepth; depth++ {
		if n.FC.HasState(nextRoot) {
			break
		}

		if !n.inflightRoots.tryAcquire(nextRoot) {
			n.log.Debug("backfill root skipped: already in-flight",
				"root", logging.LongHash(nextRoot),
				"depth", depth,
			)
			break
		}

		reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		blocks, err := reqresp.RequestBlocksByRoot(reqCtx, n.Host.P2P, pid, [][32]byte{nextRoot})
		cancel()
		n.inflightRoots.release(nextRoot)

		if err != nil || len(blocks) == 0 {
			n.log.Debug("backfill fetch failed",
				"root", logging.LongHash(nextRoot),
				"peer_id", pid.String(),
				"depth", depth,
				"err", err,
			)
			break
		}

		pending = append(pending, blocks[0])
		nextRoot = blocks[0].Message.Block.ParentRoot
	}

	if !n.FC.HasState(nextRoot) {
		n.log.Debug("backfill did not reach known ancestor",
			"parent_root", logging.LongHash(targetRoot),
			"peer_id", pid.String(),
			"fetched", len(pending),
			"stopped_at", logging.LongHash(nextRoot),
		)
		return false
	}

	// Process in forward order (oldest first).
	synced := 0
	for i := len(pending) - 1; i >= 0; i-- {
		sb := pending[i]
		blockRoot, _ := sb.Message.Block.HashTreeRoot()
		if err := n.FC.ProcessBlock(sb); err != nil {
			n.log.Debug("backfill block rejected",
				"slot", sb.Message.Block.Slot,
				"block_root", logging.LongHash(blockRoot),
				"err", err,
			)
		} else {
			synced++
		}
	}

	n.log.Info("backfill completed",
		"parent_root", logging.LongHash(targetRoot),
		"peer_id", pid.String(),
		"synced", synced,
		"fetched", len(pending),
	)
	return synced > 0
}

// ---------------------------------------------------------------------------
// syncWithPeer — refactored with depth cap and coordination guards.
// ---------------------------------------------------------------------------

// syncWithPeer exchanges status and fetches missing blocks from a single peer.
// It walks backwards from the peer's head to find blocks we're missing, then
// processes them in forward order.
func (n *Node) syncWithPeer(ctx context.Context, pid peer.ID) bool {
	if !n.peerLimiter.acquire(pid) {
		n.log.Debug("sync skipped: peer at session limit", "peer_id", pid.String())
		return false
	}
	defer n.peerLimiter.release(pid)

	status := n.FC.GetStatus()
	ourStatus := reqresp.Status{
		Finalized: &types.Checkpoint{Root: status.FinalizedRoot, Slot: status.FinalizedSlot},
		Head:      &types.Checkpoint{Root: status.Head, Slot: status.HeadSlot},
	}

	peerStatus, err := reqresp.RequestStatus(ctx, n.Host.P2P, pid, ourStatus)
	if err != nil {
		n.log.Debug("status exchange failed", "peer_id", pid.String(), "err", err)
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

	// Walk backwards: request blocks we don't have.
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

	for depth := 0; depth < maxSyncDepth; depth++ {
		if n.FC.HasState(nextRoot) {
			break
		}

		if !n.inflightRoots.tryAcquire(nextRoot) {
			n.log.Debug("sync walk root skipped: already in-flight",
				"root", logging.LongHash(nextRoot),
				"peer_id", pid.String(),
			)
			break
		}

		reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		blocks, err := reqresp.RequestBlocksByRoot(reqCtx, n.Host.P2P, pid, [][32]byte{nextRoot})
		cancel()
		n.inflightRoots.release(nextRoot)

		if err != nil || len(blocks) == 0 {
			n.log.Debug("blocks_by_root failed during sync walk",
				"peer_id", pid.String(),
				"requested_root", logging.LongHash(nextRoot),
				"err", err,
			)
			break
		}

		pending = append(pending, blocks[0])
		nextRoot = blocks[0].Message.Block.ParentRoot
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
	return synced > 0
}

// ---------------------------------------------------------------------------
// initialSync — unchanged from before. Iterates all peers on startup.
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// isBehindPeers — unchanged. Background goroutine deferred to follow-up PR.
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// processBackfillQueue — called by the ticker once per slot to resolve
// pending blocks whose parents are missing.
// ---------------------------------------------------------------------------

func (n *Node) processBackfillQueue(ctx context.Context) {
	// Drain any signals from the gossip handler.
	for {
		select {
		case <-n.backfillCh:
		default:
			goto drained
		}
	}
drained:

	// Periodic cleanup of abandoned inflight entries.
	n.inflightRoots.releaseStale(inflightStaleAge)

	missingParents := n.PendingBlocks.MissingParents()
	if len(missingParents) == 0 {
		return
	}

	recovered := 0
	for _, parentRoot := range missingParents {
		if recovered >= maxBackfillsPerTick {
			break
		}

		if n.FC.HasState(parentRoot) {
			// Parent arrived via gossip or sync — process waiting children.
			n.processPendingChildren(parentRoot, n.log)
			continue
		}

		if n.fetchParentChain(ctx, parentRoot) {
			n.processPendingChildren(parentRoot, n.log)
		}
		recovered++
	}
}
