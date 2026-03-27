package node

import (
	"context"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/geanlabs/gean/network/reqresp"
	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

func isMissingParentStateErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "parent state not found")
}

const (
	maxFetchRetries   = 10
	initialBackoffMs  = 5
	backoffMultiplier = 2
	maxFetchDepth     = 512
)

// pendingFetch tracks an in-flight block fetch with retry state.
type pendingFetch struct {
	attempts    int
	failedPeers map[peer.ID]struct{}
}

// blockFetcher manages root-targeted block fetching with dedup, backoff, and
// per-peer failure tracking. Modeled after ethlambda's P2P fetch orchestration.
type blockFetcher struct {
	mu      sync.Mutex
	pending map[[32]byte]*pendingFetch
	node    *Node
	log     *slog.Logger
}

func newBlockFetcher(n *Node) *blockFetcher {
	return &blockFetcher{
		pending: make(map[[32]byte]*pendingFetch),
		node:    n,
		log:     logging.NewComponentLogger(logging.CompNode),
	}
}

// fetchBlock requests a specific block root from the network.
// Deduplicates: if the root is already being fetched, this is a no-op.
func (bf *blockFetcher) fetchBlock(ctx context.Context, root [32]byte) {
	bf.mu.Lock()
	if _, ok := bf.pending[root]; ok {
		bf.mu.Unlock()
		return
	}
	bf.pending[root] = &pendingFetch{
		failedPeers: make(map[peer.ID]struct{}),
	}
	bf.mu.Unlock()

	go bf.fetchWithRetry(ctx, root)
}

// fetchWithRetry attempts to fetch a block root with exponential backoff.
func (bf *blockFetcher) fetchWithRetry(ctx context.Context, root [32]byte) {
	for {
		bf.mu.Lock()
		pf, ok := bf.pending[root]
		if !ok {
			bf.mu.Unlock()
			return // resolved externally
		}
		if pf.attempts >= maxFetchRetries {
			bf.log.Warn("block fetch failed after max retries",
				"root", logging.LongHash(root),
				"attempts", pf.attempts,
			)
			delete(bf.pending, root)
			bf.mu.Unlock()
			return
		}
		pf.attempts++
		attempts := pf.attempts
		failedPeers := make(map[peer.ID]struct{}, len(pf.failedPeers))
		for p := range pf.failedPeers {
			failedPeers[p] = struct{}{}
		}
		bf.mu.Unlock()

		// Backoff: 5ms, 10ms, 20ms, 40ms, ...
		if attempts > 1 {
			backoff := time.Duration(initialBackoffMs) * time.Millisecond
			for i := 1; i < attempts; i++ {
				backoff *= time.Duration(backoffMultiplier)
			}
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				bf.resolve(root)
				return
			}
		}

		// Check if resolved during backoff.
		if bf.node.FC.HasState(root) {
			bf.resolve(root)
			return
		}

		// Select a random peer, excluding failed ones.
		pid, ok := bf.selectPeer(failedPeers)
		if !ok {
			// All peers failed — reset and retry with full pool.
			bf.mu.Lock()
			if pf, ok := bf.pending[root]; ok {
				pf.failedPeers = make(map[peer.ID]struct{})
			}
			bf.mu.Unlock()
			continue
		}

		blocks, err := reqresp.RequestBlocksByRoot(ctx, bf.node.Host.P2P, pid, [][32]byte{root})
		if err != nil || len(blocks) == 0 {
			bf.mu.Lock()
			if pf, ok := bf.pending[root]; ok {
				pf.failedPeers[pid] = struct{}{}
			}
			bf.mu.Unlock()
			continue
		}

		sb := blocks[0]
		if sb == nil || sb.Message == nil || sb.Message.Block == nil {
			bf.mu.Lock()
			if pf, ok := bf.pending[root]; ok {
				pf.failedPeers[pid] = struct{}{}
			}
			bf.mu.Unlock()
			continue
		}
		blockRoot, _ := sb.Message.Block.HashTreeRoot()
		if blockRoot != root {
			bf.log.Warn("fetched block root mismatch, ignoring",
				"requested", logging.LongHash(root),
				"received", logging.LongHash(blockRoot),
				"peer_id", pid.String(),
			)
			bf.mu.Lock()
			if pf, ok := bf.pending[root]; ok {
				pf.failedPeers[pid] = struct{}{}
			}
			bf.mu.Unlock()
			continue
		}
		bf.resolve(root)

		// Process the block. If its parent is also missing, that will trigger
		// another fetchBlock call — the chain walks itself.
		if err := bf.node.FC.ProcessBlock(sb); err != nil {
			if isMissingParentStateErr(err) {
				bf.node.PendingBlocks.Add(sb)
				bf.fetchBlock(ctx, sb.Message.Block.ParentRoot)
			}
			return
		}

		bf.log.Info("fetched block",
			"slot", sb.Message.Block.Slot,
			"block_root", logging.LongHash(blockRoot),
			"peer_id", pid.String(),
		)
		bf.node.processPendingChildren(blockRoot, bf.log)
		return
	}
}

func (bf *blockFetcher) selectPeer(excluded map[peer.ID]struct{}) (peer.ID, bool) {
	peers := bf.node.Host.P2P.Network().Peers()
	if len(peers) == 0 {
		return "", false
	}

	// Filter out excluded peers.
	var pool []peer.ID
	for _, p := range peers {
		if _, failed := excluded[p]; !failed {
			pool = append(pool, p)
		}
	}
	if len(pool) == 0 {
		return "", false
	}

	return pool[rand.Intn(len(pool))], true
}

func (bf *blockFetcher) resolve(root [32]byte) {
	bf.mu.Lock()
	delete(bf.pending, root)
	bf.mu.Unlock()
}

// fetchParentChain fetches missing ancestors for a block, walking backwards
// from parentRoot up to maxFetchDepth. Returns true if the parent state became
// available (either it was already known or was fetched synchronously).
// Used by initialSync and ticker sync where we need blocking behavior.
func (n *Node) fetchParentChain(ctx context.Context, pid peer.ID, parentRoot [32]byte) bool {
	nextRoot := parentRoot
	var pending []*types.SignedBlockWithAttestation

	for i := 0; i < maxFetchDepth; i++ {
		if n.FC.HasState(nextRoot) {
			break
		}

		blocks, err := reqresp.RequestBlocksByRoot(ctx, n.Host.P2P, pid, [][32]byte{nextRoot})
		if err != nil || len(blocks) == 0 {
			break
		}

		sb := blocks[0]
		if sb == nil || sb.Message == nil || sb.Message.Block == nil {
			break
		}
		blockRoot, _ := sb.Message.Block.HashTreeRoot()
		if blockRoot != nextRoot {
			n.log.Warn("fetched block root mismatch, aborting sync walk",
				"requested", logging.LongHash(nextRoot),
				"received", logging.LongHash(blockRoot),
				"peer_id", pid.String(),
			)
			break
		}
		pending = append(pending, sb)
		nextRoot = sb.Message.Block.ParentRoot
	}

	if !n.FC.HasState(nextRoot) {
		return false
	}

	// Process in forward order (oldest first).
	for i := len(pending) - 1; i >= 0; i-- {
		if err := n.FC.ProcessBlock(pending[i]); err != nil {
			n.log.Debug("sync block rejected",
				"slot", pending[i].Message.Block.Slot,
				"err", err,
			)
		}
	}
	return true
}

// syncWithPeer exchanges status and fetches missing blocks from a single peer.
// Uses root-targeted fetch: walks from peer head backward until a known state is found.
func (n *Node) syncWithPeer(ctx context.Context, pid peer.ID) bool {
	status := n.FC.GetStatus()
	ourStatus := reqresp.Status{
		Finalized: &types.Checkpoint{Root: status.FinalizedRoot, Slot: status.FinalizedSlot},
		Head:      &types.Checkpoint{Root: status.Head, Slot: status.HeadSlot},
	}

	peerStatus, err := reqresp.RequestStatus(ctx, n.Host.P2P, pid, ourStatus)
	if err != nil || peerStatus.Head == nil || peerStatus.Finalized == nil {
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

	return n.fetchParentChain(ctx, pid, peerStatus.Head.Root)
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
