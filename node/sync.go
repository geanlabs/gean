package node

import (
	"context"
	"log/slog"
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

const (
	maxBackfillDepth     = 512
	maxConcurrentPerPeer = 2
	maxBackfillsPerTick  = 3
	maxSyncPeersPerTick  = 3
	backfillTickBudget   = 1500 * time.Millisecond
	requestTimeout       = 8 * time.Second
	inflightStaleAge     = 30 * time.Second
	statusTimeout        = 1200 * time.Millisecond
	initialSyncRounds    = maxBackfillDepth
)

type inflightRoots struct {
	mu    sync.Mutex
	roots map[[32]byte]time.Time
}

func newInflightRoots() *inflightRoots {
	return &inflightRoots{roots: make(map[[32]byte]time.Time)}
}

func (r *inflightRoots) tryAcquire(root [32]byte) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.roots[root]; exists {
		return false
	}
	r.roots[root] = time.Now()
	return true
}

func (r *inflightRoots) release(root [32]byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.roots, root)
}

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

type peerLimiter struct {
	mu       sync.Mutex
	inflight map[peer.ID]int
}

func newPeerLimiter() *peerLimiter {
	return &peerLimiter{inflight: make(map[peer.ID]int)}
}

func (l *peerLimiter) acquire(pid peer.ID) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[pid] >= maxConcurrentPerPeer {
		return false
	}
	l.inflight[pid]++
	return true
}

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

type pendingFetch struct {
	attempts    int
	failedPeers map[peer.ID]struct{}
	inFlight    bool
	queued      bool
	nextAttempt time.Time
}

type fetchCandidate struct {
	root        [32]byte
	attempts    int
	failedPeers map[peer.ID]struct{}
}

type fetchManager struct {
	mu      sync.Mutex
	pending map[[32]byte]*pendingFetch
	queue   [][32]byte
}

func newFetchManager() *fetchManager {
	return &fetchManager{pending: make(map[[32]byte]*pendingFetch)}
}

func (m *fetchManager) enqueue(root [32]byte) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.pending[root]
	if !ok {
		m.pending[root] = &pendingFetch{
			failedPeers: make(map[peer.ID]struct{}),
			queued:      true,
		}
		m.queue = append(m.queue, root)
		return true
	}
	if state.queued || state.inFlight {
		return false
	}
	state.queued = true
	m.queue = append(m.queue, root)
	return true
}

func (m *fetchManager) nextReady(limit int, now time.Time) []fetchCandidate {
	m.mu.Lock()
	defer m.mu.Unlock()

	if limit <= 0 || len(m.queue) == 0 {
		return nil
	}

	var ready []fetchCandidate
	remaining := m.queue[:0]
	for _, root := range m.queue {
		state, ok := m.pending[root]
		if !ok {
			continue
		}
		if len(ready) >= limit {
			remaining = append(remaining, root)
			continue
		}
		if state.inFlight || now.Before(state.nextAttempt) {
			remaining = append(remaining, root)
			continue
		}

		state.inFlight = true
		state.queued = false

		failedPeers := make(map[peer.ID]struct{}, len(state.failedPeers))
		for pid := range state.failedPeers {
			failedPeers[pid] = struct{}{}
		}
		ready = append(ready, fetchCandidate{
			root:        root,
			attempts:    state.attempts,
			failedPeers: failedPeers,
		})
	}
	m.queue = remaining
	return ready
}

func (m *fetchManager) markSuccess(root [32]byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.pending, root)
}

func (m *fetchManager) markRetry(root [32]byte, delay time.Duration, failedPeer peer.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.pending[root]
	if !ok {
		state = &pendingFetch{failedPeers: make(map[peer.ID]struct{})}
		m.pending[root] = state
	}
	state.inFlight = false
	state.attempts++
	if failedPeer != "" {
		state.failedPeers[failedPeer] = struct{}{}
	}
	state.nextAttempt = time.Now().Add(delay)
	if !state.queued {
		state.queued = true
		m.queue = append(m.queue, root)
	}
}

func (m *fetchManager) markDeferred(root [32]byte, delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.pending[root]
	if !ok {
		state = &pendingFetch{failedPeers: make(map[peer.ID]struct{})}
		m.pending[root] = state
	}
	state.inFlight = false
	state.nextAttempt = time.Now().Add(delay)
	if !state.queued {
		state.queued = true
		m.queue = append(m.queue, root)
	}
}

func (m *fetchManager) clearFailedPeers(root [32]byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.pending[root]
	if !ok {
		return
	}
	state.failedPeers = make(map[peer.ID]struct{})
}

func isMissingParentStateErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "parent state not found")
}

func selectRandomPeers(peers []peer.ID, exclude map[peer.ID]struct{}) []peer.ID {
	filtered := make([]peer.ID, 0, len(peers))
	for _, pid := range peers {
		if _, blocked := exclude[pid]; blocked {
			continue
		}
		filtered = append(filtered, pid)
	}
	rand.Shuffle(len(filtered), func(i, j int) {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	})
	return filtered
}

func backoffForAttempt(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	delay := 250 * time.Millisecond
	if attempt > 0 {
		delay *= time.Duration(1 << min(attempt, 5))
	}
	if delay > 30*time.Second {
		return 30 * time.Second
	}
	return delay
}

func (n *Node) resolveMissingAncestor(parentRoot [32]byte) [32]byte {
	current := parentRoot
	seen := make(map[[32]byte]struct{})
	for {
		if n.FC.HasState(current) {
			return current
		}
		if _, loop := seen[current]; loop {
			return current
		}
		seen[current] = struct{}{}

		next, ok := n.PendingBlocks.MissingAncestor(current)
		if !ok || next == current {
			return current
		}
		current = next
	}
}

func (n *Node) enqueueMissingRoot(root [32]byte) bool {
	if n.FC.HasState(root) {
		return false
	}
	enqueued := n.fetches.enqueue(root)
	if enqueued {
		select {
		case n.backfillCh <- root:
		default:
		}
	}
	return enqueued
}

func (n *Node) processOrPendBlock(
	sb *types.SignedBlockWithAttestation,
	log *slog.Logger,
) (imported bool, pending bool, err error) {
	if sb == nil || sb.Message == nil || sb.Message.Block == nil {
		return false, false, nil
	}

	block := sb.Message.Block
	blockRoot, _ := block.HashTreeRoot()

	if err := n.FC.ProcessBlock(sb); err != nil {
		if isMissingParentStateErr(err) {
			missingRoot := n.resolveMissingAncestor(block.ParentRoot)
			n.PendingBlocks.AddWithMissingAncestor(sb, missingRoot)
			n.fetches.markSuccess(blockRoot)
			n.enqueueMissingRoot(missingRoot)

			status := n.FC.GetStatus()
			log.Info("cached pending block awaiting ancestor",
				"slot", block.Slot,
				"block_root", logging.LongHash(blockRoot),
				"parent_root", logging.LongHash(block.ParentRoot),
				"missing_root", logging.LongHash(missingRoot),
				"head_slot", status.HeadSlot,
				"finalized_slot", status.FinalizedSlot,
				"pending_count", n.PendingBlocks.Len(),
			)
			return false, true, nil
		}
		n.fetches.markSuccess(blockRoot)
		return false, false, err
	}

	n.PendingBlocks.Remove(blockRoot)
	n.fetches.markSuccess(blockRoot)
	return true, false, nil
}

func (n *Node) fetchMissingRoot(ctx context.Context, cand fetchCandidate) bool {
	root := cand.root
	if n.FC.HasState(root) {
		n.fetches.markSuccess(root)
		n.processPendingChildren(root, n.log)
		return true
	}

	peers := selectRandomPeers(n.Host.P2P.Network().Peers(), cand.failedPeers)
	if len(peers) == 0 && len(cand.failedPeers) > 0 {
		n.fetches.clearFailedPeers(root)
		peers = selectRandomPeers(n.Host.P2P.Network().Peers(), nil)
	}
	if len(peers) == 0 {
		n.fetches.markDeferred(root, 250*time.Millisecond)
		n.log.Debug("missing-root fetch deferred: no eligible peers",
			"root", logging.LongHash(root),
		)
		return false
	}

	pid := peers[0]
	if !n.peerLimiter.acquire(pid) {
		n.fetches.markDeferred(root, 250*time.Millisecond)
		n.log.Debug("missing-root fetch deferred: peer at session limit",
			"root", logging.LongHash(root),
			"peer_id", pid.String(),
		)
		return false
	}
	defer n.peerLimiter.release(pid)

	if !n.inflightRoots.tryAcquire(root) {
		n.fetches.markDeferred(root, 250*time.Millisecond)
		n.log.Debug("missing-root fetch deferred: root already in-flight",
			"root", logging.LongHash(root),
			"peer_id", pid.String(),
		)
		return false
	}
	defer n.inflightRoots.release(root)

	n.log.Info("missing-root fetch started",
		"root", logging.LongHash(root),
		"peer_id", pid.String(),
		"attempt", cand.attempts+1,
	)

	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	blocks, err := reqresp.RequestBlocksByRoot(reqCtx, n.Host.P2P, pid, [][32]byte{root})
	cancel()
	if err != nil || len(blocks) == 0 {
		delay := backoffForAttempt(cand.attempts)
		n.fetches.markRetry(root, delay, pid)
		n.log.Info("missing-root fetch failed",
			"root", logging.LongHash(root),
			"peer_id", pid.String(),
			"err", err,
			"retry_in", delay,
		)
		return false
	}
	respRoot, hashErr := blocks[0].Message.Block.HashTreeRoot()
	if hashErr != nil || respRoot != root {
		delay := backoffForAttempt(cand.attempts)
		n.fetches.markRetry(root, delay, pid)
		n.log.Warn("missing-root fetch returned unexpected block",
			"requested_root", logging.LongHash(root),
			"response_root", logging.LongHash(respRoot),
			"peer_id", pid.String(),
			"hash_err", hashErr,
			"retry_in", delay,
		)
		return false
	}

	imported, pending, procErr := n.processOrPendBlock(blocks[0], n.log)
	if procErr != nil {
		n.log.Warn("fetched block rejected",
			"root", logging.LongHash(root),
			"peer_id", pid.String(),
			"err", procErr,
		)
		return false
	}
	if imported {
		n.processPendingChildren(root, n.log)
		n.log.Info("missing-root fetch imported block",
			"root", logging.LongHash(root),
			"peer_id", pid.String(),
		)
		return true
	}
	if pending {
		n.log.Info("missing-root fetch advanced ancestry search",
			"root", logging.LongHash(root),
			"peer_id", pid.String(),
		)
	}
	return pending
}

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

	peerCtx, cancel := context.WithTimeout(ctx, statusTimeout)
	peerStatus, err := reqresp.RequestStatus(peerCtx, n.Host.P2P, pid, ourStatus)
	cancel()
	if err != nil || peerStatus.Head == nil {
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

	if peerStatus.Head.Slot < status.HeadSlot {
		return false
	}
	if peerStatus.Head.Slot == status.HeadSlot && peerStatus.Head.Root == status.Head {
		return false
	}

	enqueued := n.enqueueMissingRoot(peerStatus.Head.Root)
	if enqueued {
		n.log.Info("queued peer head root for sync",
			"peer_id", pid.String(),
			"head_slot", peerStatus.Head.Slot,
			"head_root", logging.LongHash(peerStatus.Head.Root),
		)
	}
	return enqueued
}

func (n *Node) initialSync(ctx context.Context) {
	peers := n.Host.P2P.Network().Peers()
	n.log.Info("initial sync starting", "peer_count", len(peers))
	for _, pid := range peers {
		n.syncWithPeer(ctx, pid)
	}
	for rounds := 0; rounds < initialSyncRounds; rounds++ {
		if n.processBackfillQueue(ctx, 0) == 0 {
			break
		}
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
		peerCtx, cancel := context.WithTimeout(ctx, statusTimeout)
		peerStatus, err := reqresp.RequestStatus(peerCtx, n.Host.P2P, pid, ourStatus)
		cancel()
		if err != nil || peerStatus.Head == nil {
			continue
		}
		if peerStatus.Head.Slot > maxPeerHeadSlot {
			maxPeerHeadSlot = peerStatus.Head.Slot
		}
	}

	return status.HeadSlot < maxPeerHeadSlot, maxPeerHeadSlot
}

func (n *Node) processBackfillQueue(ctx context.Context, budget time.Duration) int {
	if budget > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, budget)
		defer cancel()
	}

	for {
		select {
		case <-n.backfillCh:
		default:
			goto drained
		}
	}
drained:

	n.inflightRoots.releaseStale(inflightStaleAge)

	for _, root := range n.PendingBlocks.MissingParents() {
		if n.FC.HasState(root) {
			n.fetches.markSuccess(root)
			n.processPendingChildren(root, n.log)
			continue
		}
		n.enqueueMissingRoot(root)
	}

	candidates := n.fetches.nextReady(maxBackfillsPerTick, time.Now())
	processed := 0
	for i, cand := range candidates {
		if budget > 0 && ctx.Err() != nil {
			for _, remaining := range candidates[i:] {
				n.fetches.markDeferred(remaining.root, 0)
			}
			n.log.Debug("backfill queue budget exhausted",
				"budget", budget,
				"processed", processed,
				"remaining", len(candidates)-processed,
			)
			break
		}
		if n.FC.HasState(cand.root) {
			n.fetches.markSuccess(cand.root)
			n.processPendingChildren(cand.root, n.log)
			processed++
			continue
		}
		n.fetchMissingRoot(ctx, cand)
		processed++
	}
	return processed
}
