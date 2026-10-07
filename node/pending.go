package node

import (
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/types"
)

func (e *Engine) bufferMissingParentBlock(
	signedBlock *types.SignedBlock,
	blockRoot [32]byte,
	parentRoot [32]byte,
	queue *[]*types.SignedBlock,
) {
	block := signedBlock.Block
	if e.pendingBlocks.Count() >= MaxPendingBlocks && !e.evictFarthestPending(block.Slot) {
		logger.Warn(logger.Chain, "pending block cache full (%d), rejecting block slot=%d block_root=0x%x",
			MaxPendingBlocks, block.Slot, blockRoot)
		return
	}

	depth := 1
	if parentDepth, ok := e.pendingBlocks.Depth(parentRoot); ok {
		depth = parentDepth + 1
	}
	if depth > MaxBlockFetchDepth {
		logger.Warn(logger.Chain, "block fetch depth exceeded (%d > %d), discarding block slot=%d block_root=0x%x",
			depth, MaxBlockFetchDepth, block.Slot, blockRoot)
		return
	}

	logger.Warn(logger.Chain, "block parent missing slot=%d block_root=0x%x parent_root=0x%x depth=%d, storing as pending",
		block.Slot, blockRoot, parentRoot, depth)

	e.pendingBlocks.SetDepth(blockRoot, depth)
	e.pendingBlocks.SetSlot(blockRoot, block.Slot)
	missingRoot := e.pendingBlocks.ResolveAncestor(parentRoot)
	e.pendingBlocks.SetParent(blockRoot, parentRoot)
	e.store.StorePendingBlock(blockRoot, signedBlock)
	e.pendingBlocks.AddChild(parentRoot, blockRoot)

	missingRoot, queued := e.queueStoredAncestor(missingRoot, queue)
	if queued {
		return
	}
	e.queueMissingBlockFetch(missingRoot)
}

// evictFarthestPending frees a slot in the full pending buffer for a block at
// incomingSlot by discarding the entry farthest from the known chain. A full
// buffer must not reject blocks near the connect frontier: those are the only
// ones that can turn into imports, and turning them away is how a lagging
// node's buffer stays poisoned with an unconnectable tail forever. When the
// incoming block sits no closer than everything already held, it is the one
// that loses.
func (e *Engine) evictFarthestPending(incomingSlot uint64) bool {
	evictRoot, evictSlot, ok := e.pendingBlocks.HighestSlotEntry()
	if !ok || evictSlot <= incomingSlot {
		return false
	}
	logger.Warn(logger.Chain, "pending block cache full (%d), evicting slot=%d block_root=0x%x to admit slot=%d",
		MaxPendingBlocks, evictSlot, evictRoot, incomingSlot)
	e.pendingBlocks.DiscardSubtree(evictRoot)
	return true
}

func (e *Engine) queueStoredAncestor(missingRoot [32]byte, queue *[]*types.SignedBlock) ([32]byte, bool) {
	for {
		header := e.store.GetBlockHeader(missingRoot)
		if header == nil {
			return missingRoot, false
		}
		if e.store.HasState(header.ParentRoot) {
			if storedBlock := e.store.GetSignedBlock(missingRoot); storedBlock != nil {
				*queue = append(*queue, storedBlock)
			}
			return missingRoot, true
		}
		e.pendingBlocks.AddChild(header.ParentRoot, missingRoot)
		e.pendingBlocks.SetParent(missingRoot, header.ParentRoot)
		missingRoot = header.ParentRoot
	}
}

func (e *Engine) collectPendingChildren(parentRoot [32]byte, queue *[]*types.SignedBlock) {
	childRoots, ok := e.pendingBlocks.RemoveBucket(parentRoot)
	if !ok {
		return
	}

	logger.Info(logger.Chain, "processing %d pending children of parent_root=0x%x", len(childRoots), parentRoot)

	for childRoot := range childRoots {
		e.pendingBlocks.ClearEntry(childRoot)

		childBlock := e.store.GetSignedBlock(childRoot)
		if childBlock == nil {
			logger.Warn(logger.Chain, "pending block block_root=0x%x missing from DB, skipping", childRoot)
			continue
		}
		*queue = append(*queue, childBlock)
	}
}

func (e *Engine) discardFinalizedPending(finalizedSlot uint64) {
	discarded := 0

	for _, pair := range e.pendingBlocks.Pairs() {
		parentRoot, childRoot := pair[0], pair[1]
		header := e.store.GetBlockHeader(childRoot)
		if header != nil && header.Slot <= finalizedSlot {
			e.pendingBlocks.DiscardSubtree(childRoot)
			e.pendingBlocks.RemoveChild(parentRoot, childRoot)
			discarded++
		}
	}

	if discarded > 0 {
		logger.Info(logger.Store, "discarded %d finalized pending blocks (finalized_slot=%d)", discarded, finalizedSlot)
	}

	if removed := e.pendingAttestations.PruneBelow(finalizedSlot); removed > 0 {
		logger.Info(logger.Store, "discarded %d finalized pending attestations (finalized_slot=%d)", removed, finalizedSlot)
	}
}

func (e *Engine) onFailedRoot(failedRoot [32]byte) {
	// The fetch for this root is exhausted; drop its in-flight marker so it can be
	// re-requested if a later child reintroduces the gap.
	delete(e.fetchInFlight, failedRoot)

	children, ok := e.pendingBlocks.RemoveBucket(failedRoot)
	if !ok {
		return
	}

	discarded := 0
	for childRoot := range children {
		e.pendingBlocks.DiscardSubtree(childRoot)
		discarded++
	}
	logger.Warn(logger.Sync, "fetch exhausted for root 0x%x, discarded %d pending child block(s)", failedRoot, discarded)
}
