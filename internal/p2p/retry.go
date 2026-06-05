package p2p

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/types"
)

const (
	MaxFetchRetries     = 10
	InitialBackoffMs    = 5
	BackoffMultiplier   = 2
	MaxBlocksPerRequest = 10
)

type SignedBlockResult struct {
	Root  [32]byte
	Block []*types.SignedBlock
	Err   error
}

func (h *Host) FetchBlocksByRootWithRetry(ctx context.Context, roots [][32]byte) ([]*SignedBlockResult, error) {
	results := make([]*SignedBlockResult, 0, len(roots))
	for _, root := range roots {
		block, err := h.fetchSingleBlockWithRetry(ctx, root)
		results = append(results, &SignedBlockResult{
			Root:  root,
			Block: block,
			Err:   err,
		})
	}
	return results, nil
}

func (h *Host) FetchBlocksByRootBatchWithRetry(ctx context.Context, roots [][32]byte) ([]*types.SignedBlock, [][32]byte, error) {
	if len(roots) == 0 {
		return nil, nil, nil
	}
	if len(roots) > MaxBlocksPerRequest {
		roots = roots[:MaxBlocksPerRequest]
	}

	excluded := make(map[peer.ID]bool)
	backoff := time.Duration(InitialBackoffMs) * time.Millisecond

	for attempt := range MaxFetchRetries {
		peerID := h.peerStore.RandomPeer(excluded)
		if peerID == "" {
			return nil, roots, fmt.Errorf("no peers available for batch block fetch")
		}

		blocks, err := h.FetchBlocksByRoot(ctx, peerID, roots)
		if err == nil && len(blocks) > 0 {
			return blocks, computeMissingRoots(roots, blocks), nil
		}

		excluded[peerID] = true
		h.logFetchFailure("batch block fetch", attempt, peerID, err, "root_count", len(roots))

		select {
		case <-ctx.Done():
			return nil, roots, ctx.Err()
		case <-time.After(backoff):
			backoff *= BackoffMultiplier
		}
	}

	return nil, roots, fmt.Errorf("batch block fetch failed after %d retries for %d roots", MaxFetchRetries, len(roots))
}

func (h *Host) FetchBlocksByRangeWithRetry(
	ctx context.Context,
	startSlot, count uint64,
) ([]*types.SignedBlock, error) {
	if err := validateRangeRequest(startSlot, count); err != nil {
		return nil, err
	}

	excluded := make(map[peer.ID]bool)
	backoff := time.Duration(InitialBackoffMs) * time.Millisecond

	for attempt := range MaxFetchRetries {
		peerID := h.peerStore.RandomPeer(excluded)
		if peerID == "" {
			return nil, fmt.Errorf("no peers available for blocks_by_range fetch")
		}

		blocks, err := h.FetchBlocksByRange(ctx, peerID, startSlot, count)
		if err == nil && len(blocks) > 0 {
			return blocks, nil
		}

		excluded[peerID] = true
		h.logFetchFailure("blocks_by_range fetch", attempt, peerID, err, "start_slot", startSlot)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
			backoff *= BackoffMultiplier
		}
	}

	return nil, fmt.Errorf("blocks_by_range fetch failed after %d retries (start_slot=%d, count=%d)", MaxFetchRetries, startSlot, count)
}

func (h *Host) fetchSingleBlockWithRetry(ctx context.Context, root [32]byte) ([]*types.SignedBlock, error) {
	excluded := make(map[peer.ID]bool)
	backoff := time.Duration(InitialBackoffMs) * time.Millisecond

	for attempt := range MaxFetchRetries {
		peerID := h.peerStore.RandomPeer(excluded)
		if peerID == "" {
			return nil, fmt.Errorf("no peers available for block fetch")
		}

		blocks, err := h.FetchBlocksByRoot(ctx, peerID, [][32]byte{root})
		if err == nil && len(blocks) > 0 {
			return blocks, nil
		}

		excluded[peerID] = true
		h.logFetchFailure("block fetch", attempt, peerID, err, "block_root", fmt.Sprintf("0x%x", root))

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
			backoff *= BackoffMultiplier
		}
	}

	return nil, fmt.Errorf("block fetch failed after %d retries for %x", MaxFetchRetries, root)
}

func computeMissingRoots(requested [][32]byte, delivered []*types.SignedBlock) [][32]byte {
	deliveredRoots := make(map[[32]byte]bool, len(delivered))
	for _, block := range delivered {
		if block == nil || block.Block == nil {
			continue
		}
		root, err := block.Block.HashTreeRoot()
		if err != nil {
			continue
		}
		deliveredRoots[root] = true
	}

	missing := make([][32]byte, 0, len(requested))
	for _, root := range requested {
		if !deliveredRoots[root] {
			missing = append(missing, root)
		}
	}
	return missing
}

func (h *Host) logFetchFailure(label string, attempt int, peerID peer.ID, err error, key string, value any) {
	reason := "peer returned no blocks"
	if err != nil {
		reason = err.Error()
	}
	logger.Warn(logger.Network, "%s attempt %d/%d failed peer=%s %s=%v reason=%s",
		label, attempt+1, MaxFetchRetries, peerID, key, value, reason)
}
