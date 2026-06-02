package node

import (
	"time"

	"github.com/geanlabs/gean/internal/blockprocessor"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// onBlock processes a received block using an iterative work queue.
func (e *Engine) onBlock(signedBlock *types.SignedBlock) {
	oldFinalizedSlot := e.Store.LatestFinalized().Slot

	queue := []*types.SignedBlock{signedBlock}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		e.processOneBlock(current, &queue)
	}

	// Prune AFTER the entire cascade completes — not mid-cascade.
	newFinalized := e.Store.LatestFinalized()
	if newFinalized.Slot > oldFinalizedSlot {
		store.PruneOnFinalization(e.Store, e.FC, oldFinalizedSlot, newFinalized.Slot, newFinalized.Root)
		e.discardFinalizedPending(newFinalized.Slot)
	}
}

func (e *Engine) processOneBlock(signedBlock *types.SignedBlock, queue *[]*types.SignedBlock) {
	block := signedBlock.Block
	blockRoot, _ := block.HashTreeRoot()
	parentRoot := block.ParentRoot

	// Skip if already processed.
	if e.Store.HasState(blockRoot) {
		return
	}

	// Reject blocks strictly below the finalized slot: their parent state has
	// been pruned and they cannot extend the canonical chain.
	finalizedSlot := e.Store.LatestFinalized().Slot
	if block.Slot < finalizedSlot {
		logger.Warn(logger.Chain, "rejecting pre-finalized block slot=%d block_root=0x%x finalized_slot=%d",
			block.Slot, blockRoot, finalizedSlot)
		return
	}

	hasParent := e.Store.HasState(parentRoot)
	logger.Info(logger.Chain, "processing block slot=%d block_root=0x%x has_parent=%t", block.Slot, blockRoot, hasParent)

	// Check if parent state exists.
	if !hasParent {
		// Check pending block cache limit.
		if e.Pending.Count() >= MaxPendingBlocks {
			logger.Warn(logger.Chain, "pending block cache full (%d), rejecting block slot=%d block_root=0x%x",
				MaxPendingBlocks, block.Slot, blockRoot)
			return
		}

		// Compute depth: parent's depth + 1.
		depth := 1
		if parentDepth, ok := e.Pending.Depth(parentRoot); ok {
			depth = parentDepth + 1
		}

		// Check depth limit.
		if depth > MaxBlockFetchDepth {
			logger.Warn(logger.Chain, "block fetch depth exceeded (%d > %d), discarding block slot=%d block_root=0x%x",
				depth, MaxBlockFetchDepth, block.Slot, blockRoot)
			return
		}

		logger.Warn(logger.Chain, "block parent missing slot=%d block_root=0x%x parent_root=0x%x depth=%d, storing as pending",
			block.Slot, blockRoot, parentRoot, depth)

		// Track depth.
		e.Pending.SetDepth(blockRoot, depth)

		// Resolve the actual missing ancestor by walking the chain.
		missingRoot := e.Pending.ResolveAncestor(parentRoot)

		e.Pending.SetParent(blockRoot, missingRoot)

		// Store block in DB as pending (no LiveChain entry — invisible to fork choice).
		e.Store.StorePendingBlock(blockRoot, signedBlock)

		// Track parent→child relationship in memory.
		e.Pending.AddChild(parentRoot, blockRoot)

		// Walk up through DB: if missingRoot has a stored header,
		// the actual missing block is further up.
		for {
			header := e.Store.GetBlockHeader(missingRoot)
			if header == nil {
				break // truly missing — request from network
			}
			if e.Store.HasState(header.ParentRoot) {
				// Parent state available — load and enqueue for processing.
				storedBlock := e.Store.GetSignedBlock(missingRoot)
				if storedBlock != nil {
					*queue = append(*queue, storedBlock)
				}
				return
			}
			// Block exists but parent state missing — register as pending.
			e.Pending.AddChild(header.ParentRoot, missingRoot)
			e.Pending.SetParent(missingRoot, header.ParentRoot)
			missingRoot = header.ParentRoot
		}

		// Request the actual missing block from network via the fetch batcher.
		if e.P2P != nil {
			logger.Info(logger.Sync, "queueing missing block block_root=0x%x for batched fetch", missingRoot)
			select {
			case e.FetchRootCh <- missingRoot:
			default:
				logger.Warn(logger.Sync, "fetch root channel full, dropping request for 0x%x", missingRoot)
			}
		}
		return
	}

	// Parent exists — process the block.
	blockStart := time.Now()
	err := blockprocessor.OnBlock(e.Store, signedBlock)
	metrics.ObserveBlockProcessingTime(time.Since(blockStart).Seconds())
	if err != nil {
		logger.Error(logger.Chain, "block processing failed slot=%d block_root=0x%x: %v", block.Slot, blockRoot, err)
		return
	}

	// Register in fork choice.
	e.FC.OnBlock(block.Slot, blockRoot, parentRoot)

	// Check for finalization advance.
	finalized := e.Store.LatestFinalized()
	if finalized.Slot > 0 {
		e.FC.Prune(finalized.Root)
	}

	// Update head.
	e.updateHead()

	// Clear depth tracking for this block (now processed).
	e.Pending.ClearDepth(blockRoot)

	// Replay any gossip attestations that were buffered awaiting this exact
	// head block. Fires for every successful import — cascaded children
	// (added via collectPendingChildren below) re-enter processOneBlock and
	// run their own replay pass, so a multi-block chain repair drains the
	// buffer for each block in order. Replay fans out to one goroutine per
	// attestation, matching the existing live-gossip pattern in Run().
	e.replayPendingAttestations(blockRoot)

	// Cascade: enqueue pending children for processing.
	e.collectPendingChildren(blockRoot, queue)
}

// replayPendingAttestations drains every gossip attestation that was buffered
// awaiting this head block and fires each one back through onGossipAttestation.
// Each replay runs in its own goroutine because XMSS verification is ~500ms
// per attestation and would otherwise serialize behind the engine main loop.
func (e *Engine) replayPendingAttestations(headRoot [32]byte) {
	pending := e.PendingAttestations.Drain(headRoot)
	if len(pending) == 0 {
		return
	}
	logger.Info(logger.Gossip, "replaying %d buffered attestations for newly arrived head=0x%x",
		len(pending), headRoot)
	for _, att := range pending {
		att := att
		go e.onGossipAttestation(att)
	}
}

// collectPendingChildren moves pending children of parent into the work queue.
func (e *Engine) collectPendingChildren(parentRoot [32]byte, queue *[]*types.SignedBlock) {
	childRoots, ok := e.Pending.RemoveBucket(parentRoot)
	if !ok {
		return
	}

	logger.Info(logger.Chain, "processing %d pending children of parent_root=0x%x", len(childRoots), parentRoot)

	for childRoot := range childRoots {
		e.Pending.ClearEntry(childRoot)

		childBlock := e.Store.GetSignedBlock(childRoot)
		if childBlock == nil {
			logger.Warn(logger.Chain, "pending block block_root=0x%x missing from DB, skipping", childRoot)
			continue
		}
		*queue = append(*queue, childBlock)
	}
}

// discardFinalizedPending removes all pending blocks at or below the finalized slot.
// Their subtrees are also discarded since they can never be processed.
func (e *Engine) discardFinalizedPending(finalizedSlot uint64) {
	discarded := 0

	for _, pair := range e.Pending.Pairs() {
		parentRoot, childRoot := pair[0], pair[1]
		header := e.Store.GetBlockHeader(childRoot)
		if header != nil && header.Slot <= finalizedSlot {
			// This pending block is at/below finalized — discard entire subtree.
			e.Pending.DiscardSubtree(childRoot)
			e.Pending.RemoveChild(parentRoot, childRoot)
			discarded++
		}
	}

	if discarded > 0 {
		logger.Info(logger.Store, "discarded %d finalized pending blocks (finalized_slot=%d)", discarded, finalizedSlot)
	}

	// Drop buffered attestations whose target slot is at or below the new
	// finalized slot — their head block (if it ever arrives) will likewise
	// be too old to act on, so the attestation can never be replayed.
	if removed := e.PendingAttestations.PruneBelow(finalizedSlot); removed > 0 {
		logger.Info(logger.Store, "discarded %d finalized pending attestations (finalized_slot=%d)", removed, finalizedSlot)
	}
}

// drainPendingBlocks processes all queued blocks from the channel before
// attestation production. Ensures the node's head reflects the latest blocks
// so attestation targets/sources match across nodes.
func (e *Engine) drainPendingBlocks() {
	drained := 0
	for {
		select {
		case block := <-e.BlockCh:
			e.onBlock(block)
			drained++
		default:
			if drained > 0 {
				logger.Info(logger.Chain, "drained %d pending blocks before attestation", drained)
			}
			return
		}
	}
}
