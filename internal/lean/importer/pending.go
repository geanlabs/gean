package importer

import (
	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/lean/types"
)

func (i *Importer) bufferMissingParentBlock(
	signedBlock *types.SignedBlock,
	blockRoot [32]byte,
	parentRoot [32]byte,
	queue *[]*types.SignedBlock,
) {
	block := signedBlock.Block
	// A pending block is written to disk before its signature can be checked, so
	// first turn away what import would reject anyway: a block past the future
	// horizon, or one whose proposer is not the slot's. The registry is fixed at
	// genesis, so the head's validator count holds.
	if currentSlot := i.store.Time() / types.IntervalsPerSlot; types.BeyondFutureHorizon(block.Slot, currentSlot) {
		logger.Warn(logger.Chain, "rejecting pending block beyond future horizon slot=%d current_slot=%d block_root=0x%x",
			block.Slot, currentSlot, blockRoot)
		return
	}
	keys := i.store.ValidatorKeys(i.store.Head())
	if keys == nil {
		logger.Error(logger.Chain, "rejecting pending block: head state has no validators slot=%d block_root=0x%x",
			block.Slot, blockRoot)
		return
	}
	if !types.IsProposer(block.Slot, block.ProposerIndex, keys.Len()) {
		logger.Warn(logger.Chain, "rejecting pending block with wrong proposer slot=%d proposer=%d block_root=0x%x",
			block.Slot, block.ProposerIndex, blockRoot)
		return
	}
	if i.Pending.Count() >= MaxPendingBlocks && !i.evictFarthestPending(block.Slot) {
		logger.Warn(logger.Chain, "pending block cache full (%d), rejecting block slot=%d block_root=0x%x",
			MaxPendingBlocks, block.Slot, blockRoot)
		return
	}

	depth := 1
	if parentDepth, ok := i.Pending.Depth(parentRoot); ok {
		depth = parentDepth + 1
	}
	if depth > MaxBlockFetchDepth {
		logger.Warn(logger.Chain, "block fetch depth exceeded (%d > %d), discarding block slot=%d block_root=0x%x",
			depth, MaxBlockFetchDepth, block.Slot, blockRoot)
		return
	}

	logger.Warn(logger.Chain, "block parent missing slot=%d block_root=0x%x parent_root=0x%x depth=%d, storing as pending",
		block.Slot, blockRoot, parentRoot, depth)

	// The pending set must only name blocks that are on disk. StorePendingBlock
	// logs its own failure.
	if err := i.store.StorePendingBlock(blockRoot, signedBlock); err != nil {
		return
	}
	i.Pending.SetDepth(blockRoot, depth)
	i.Pending.SetSlot(blockRoot, block.Slot)
	missingRoot := i.Pending.ResolveAncestor(parentRoot)
	i.Pending.SetParent(blockRoot, parentRoot)
	i.Pending.MarkStored(blockRoot)
	i.Pending.AddChild(parentRoot, blockRoot)

	missingRoot, handled := i.queueStoredAncestor(missingRoot, queue)
	if handled {
		return
	}
	i.queueMissingBlockFetch(missingRoot)
}

// evictFarthestPending frees a slot in the full pending buffer for a block at
// incomingSlot by discarding the entry farthest from the known chain. A full
// buffer must not reject blocks near the connect frontier: those are the only
// ones that can turn into imports, and turning them away is how a lagging
// node's buffer stays poisoned with an unconnectable tail forever. When the
// incoming block sits no closer than everything already held, it is the one
// that loses.
func (i *Importer) evictFarthestPending(incomingSlot uint64) bool {
	evictRoot, evictSlot, ok := i.Pending.HighestSlotEntry()
	if !ok || evictSlot <= incomingSlot {
		return false
	}
	logger.Warn(logger.Chain, "pending block cache full (%d), evicting slot=%d block_root=0x%x to admit slot=%d",
		MaxPendingBlocks, evictSlot, evictRoot, incomingSlot)
	i.discardPending(evictRoot)
	return true
}

// discardPending drops a pending subtree and deletes from disk the blocks
// bufferMissingParentBlock stored and that were not imported since. Imported
// blocks below finality also lack a state, so a state check cannot tell them apart.
func (i *Importer) discardPending(root [32]byte) {
	i.store.DeletePendingBlocks(i.Pending.DiscardSubtree(root))
}

// queueStoredAncestor walks up stored headers from missingRoot. It reports true
// when the walk is handled here: the first block whose parent has a state is
// queued for import, or the walk reached a stateless block at or below
// finality, which nothing above can import through, and the subtree is dropped.
// A walk past MaxBlockFetchDepth stored blocks is dropped too: a gap that deep
// is range sync's to close, not the dispatch loop's.
func (i *Importer) queueStoredAncestor(missingRoot [32]byte, queue *[]*types.SignedBlock) ([32]byte, bool) {
	finalizedSlot := i.store.LatestFinalized().Slot
	for steps := 0; ; steps++ {
		slot, parent, ok := i.store.BlockSlotAndParent(missingRoot)
		if !ok {
			return missingRoot, false
		}
		if i.store.HasState(parent) {
			if storedBlock := i.store.GetSignedBlock(missingRoot); storedBlock != nil {
				*queue = append(*queue, storedBlock)
			}
			return missingRoot, true
		}
		if slot <= finalizedSlot {
			logger.Warn(logger.Chain, "pending chain reaches stateless block at or below finality slot=%d block_root=0x%x finalized_slot=%d, discarding",
				slot, missingRoot, finalizedSlot)
			i.discardPending(missingRoot)
			return missingRoot, true
		}
		if steps >= MaxBlockFetchDepth {
			logger.Warn(logger.Chain, "stored ancestor walk exceeded %d blocks slot=%d block_root=0x%x, discarding",
				MaxBlockFetchDepth, slot, missingRoot)
			i.discardPending(missingRoot)
			return missingRoot, true
		}
		// Every linked entry carries its slot, so eviction and the finalization
		// sweep can see it.
		i.Pending.SetSlot(missingRoot, slot)
		i.Pending.AddChild(parent, missingRoot)
		i.Pending.SetParent(missingRoot, parent)
		missingRoot = parent
	}
}

func (i *Importer) collectPendingChildren(parentRoot [32]byte, queue *[]*types.SignedBlock) {
	childRoots, ok := i.Pending.RemoveBucket(parentRoot)
	if !ok {
		return
	}

	logger.Info(logger.Chain, "processing %d pending children of parent_root=0x%x", len(childRoots), parentRoot)

	for childRoot := range childRoots {
		i.Pending.ClearEntry(childRoot)

		childBlock := i.store.GetSignedBlock(childRoot)
		if childBlock == nil {
			logger.Warn(logger.Chain, "pending block block_root=0x%x missing from DB, skipping", childRoot)
			continue
		}
		*queue = append(*queue, childBlock)
	}
}

// DiscardFinalized drops pending blocks and attestations at or below the newly
// finalized slot: none of them can become canonical.
func (i *Importer) DiscardFinalized(finalizedSlot uint64) {
	discarded, stored := i.Pending.DiscardAtOrBelow(finalizedSlot)
	i.store.DeletePendingBlocks(stored)
	if discarded > 0 {
		logger.Info(logger.Store, "discarded %d finalized pending blocks (finalized_slot=%d)", discarded, finalizedSlot)
	}

	if removed := i.pendingAttestations.PruneBelow(finalizedSlot); removed > 0 {
		logger.Info(logger.Store, "discarded %d finalized pending attestations (finalized_slot=%d)", removed, finalizedSlot)
	}
}

// OnFailedRoot handles a root whose fetch was exhausted: the pending blocks
// waiting on it can never connect.
func (i *Importer) OnFailedRoot(failedRoot [32]byte) {
	// The fetch for this root is exhausted; drop its in-flight marker so it can be
	// re-requested if a later child reintroduces the gap.
	delete(i.fetchInFlight, failedRoot)

	children, ok := i.Pending.RemoveBucket(failedRoot)
	if !ok {
		return
	}

	discarded := 0
	for childRoot := range children {
		i.discardPending(childRoot)
		discarded++
	}
	logger.Warn(logger.Sync, "fetch exhausted for root 0x%x, discarded %d pending child block(s)", failedRoot, discarded)
}
