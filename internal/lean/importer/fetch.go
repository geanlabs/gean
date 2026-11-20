package importer

import (
	"context"
	"time"

	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/lean/p2p"
	"github.com/geanlabs/gean/internal/lean/types"
)

const fetchBatchGracePeriod = 50 * time.Millisecond

// RunFetcher batches missing-parent roots and fetches them from peers until ctx
// ends. It runs on its own goroutine.
func (i *Importer) RunFetcher(ctx context.Context) {
	for {
		var batch [][32]byte
		seen := make(map[[32]byte]bool)

		select {
		case <-ctx.Done():
			return
		case root := <-i.fetchRoots:
			batch = append(batch, root)
			seen[root] = true
		}

		grace := time.After(fetchBatchGracePeriod)
	gather:
		for len(batch) < p2p.MaxBlocksPerRequest {
			select {
			case <-ctx.Done():
				return
			case root := <-i.fetchRoots:
				if !seen[root] {
					batch = append(batch, root)
					seen[root] = true
				}
			case <-grace:
				break gather
			}
		}

		i.fireBatchFetch(ctx, batch)
	}
}

func (i *Importer) fireBatchFetch(ctx context.Context, roots [][32]byte) {
	if i.fetcher == nil || len(roots) == 0 {
		return
	}
	logger.Info(logger.Sync, "batched fetch starting count=%d", len(roots))
	blocks, missing, err := i.fetcher.FetchBlocksByRootBatchWithRetry(ctx, roots)
	if err != nil {
		logger.Warn(logger.Sync, "batched fetch failed count=%d err=%v", len(roots), err)
	}
	// Fetched parents fill gaps the dispatch loop is waiting on; deliver with
	// backpressure so none are dropped while their in-flight markers say the
	// fetch succeeded.
	for _, b := range blocks {
		if !i.SubmitSync(ctx, b) {
			return
		}
	}
	i.notifyFailedRoots(ctx, missing)
}

// notifyFailedRoots hands exhausted roots to the dispatch loop, blocking until each
// is accepted. Dropping one is not a lost log line: a root's in-flight marker clears
// only when its block arrives or this notification is handled, so a dropped root is
// never re-requested and its gap never closes. Range backfill cannot cover for it —
// that only runs against a peer whose head is ahead, which is untrue of a node
// marooned on its own fork. The marker map belongs to the dispatch loop, so the
// batcher cannot clear it here; backpressure is the only safe option, and the loop
// never sends on this channel, so blocking cannot deadlock.
func (i *Importer) notifyFailedRoots(ctx context.Context, roots [][32]byte) {
	for _, r := range roots {
		select {
		case i.failedRoots <- r:
		case <-ctx.Done():
			return
		}
	}
}

func (i *Importer) queueMissingBlockFetch(root [32]byte) {
	if i.fetcher == nil {
		return
	}
	// The zero root is genesis's parent: there is no such block to fetch.
	if types.IsZeroRoot(root) {
		return
	}
	// A missing parent is re-derived on every child that arrives referencing it, so the
	// same root would otherwise be queued hundreds of times and saturate the fetch queue with
	// duplicates — starving the fetch and freezing the head while far behind. Queue each
	// root at most once until its block is received or its fetch is exhausted.
	if i.fetchInFlight[root] {
		return
	}
	select {
	case i.fetchRoots <- root:
		i.fetchInFlight[root] = true
		logger.Info(logger.Sync, "queueing missing block block_root=0x%x for batched fetch", root)
	default:
		logger.Warn(logger.Sync, "fetch root channel full, dropping request for 0x%x", root)
	}
}
