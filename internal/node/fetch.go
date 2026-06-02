package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/p2p"
)

// fetchBatchGracePeriod is how long the batcher waits for additional roots
// to coalesce after receiving the first one.
const fetchBatchGracePeriod = 50 * time.Millisecond

// runFetchBatcher coalesces fetch requests from FetchRootCh into batches of
// up to MaxBlocksPerRequest roots, then fires a single batched fetch per batch.
//
// This drastically reduces network round-trips during catch-up: instead of
// 100 sequential requests for 100 missing blocks, we make ~10 requests with
// 10 roots each. The grace period (50ms) gives time for closely-spaced
// fetch needs to coalesce without delaying steady-state operation noticeably.
func (e *Engine) runFetchBatcher(ctx context.Context) {
	for {
		var batch [][32]byte
		seen := make(map[[32]byte]bool)

		// Wait for the first root (blocks indefinitely).
		select {
		case <-ctx.Done():
			return
		case root := <-e.FetchRootCh:
			batch = append(batch, root)
			seen[root] = true
		}

		// Collect more roots within the grace period, up to MaxBlocksPerRequest.
		grace := time.After(fetchBatchGracePeriod)
	gather:
		for len(batch) < p2p.MaxBlocksPerRequest {
			select {
			case <-ctx.Done():
				return
			case root := <-e.FetchRootCh:
				if !seen[root] {
					batch = append(batch, root)
					seen[root] = true
				}
			case <-grace:
				break gather
			}
		}

		e.fireBatchFetch(ctx, batch)
	}
}

// fireBatchFetch issues a batched blocks_by_root request and feeds the
// returned blocks back into the engine. Roots not delivered are reported
// as failed so their pending subtrees can be discarded.
func (e *Engine) fireBatchFetch(ctx context.Context, roots [][32]byte) {
	if e.P2P == nil || len(roots) == 0 {
		return
	}
	logger.Info(logger.Sync, "batched fetch starting count=%d", len(roots))
	blocks, missing, err := e.P2P.FetchBlocksByRootBatchWithRetry(ctx, roots)
	if err != nil {
		logger.Warn(logger.Sync, "batched fetch failed count=%d err=%v", len(roots), err)
	}
	for _, b := range blocks {
		e.OnBlock(b)
	}
	for _, r := range missing {
		select {
		case e.FailedRootCh <- r:
		default:
			logger.Warn(logger.Sync, "failed root channel full, dropping notification for 0x%x", r)
		}
	}
}

// onFailedRoot discards pending blocks whose subtree depends on a root that
// no peer could serve after exhausting all fetch retries.
//
// We free memory by dropping the orphaned subtree, but we do NOT permanently
// blacklist the root — if a peer reconnects with the missing block later, or
// a new orphan arrives needing the same parent, gean will try fetching again.
func (e *Engine) onFailedRoot(failedRoot [32]byte) {
	children, ok := e.Pending.RemoveBucket(failedRoot)
	if !ok {
		return
	}

	discarded := 0
	for childRoot := range children {
		e.Pending.DiscardSubtree(childRoot)
		discarded++
	}
	logger.Warn(logger.Sync, "fetch exhausted for root 0x%x, discarded %d pending child block(s)", failedRoot, discarded)
}
