package node

import (
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/types"
)

func (e *Engine) bufferMissingParentBlock(
	signedBlock *types.SignedBlock,
	blockRoot [32]byte,
	parentRoot [32]byte,
	queue *[]*types.SignedBlock,
) {
	block := signedBlock.Block
	if e.Pending.Count() >= MaxPendingBlocks && !e.evictFarthestPending(block.Slot) {
		logger.Warn(logger.Chain, "pending block cache full (%d), rejecting block slot=%d block_root=0x%x",
			MaxPendingBlocks, block.Slot, blockRoot)
		return
	}

	depth := 1
	if parentDepth, ok := e.Pending.Depth(parentRoot); ok {
		depth = parentDepth + 1
	}
	if depth > MaxBlockFetchDepth {
		logger.Warn(logger.Chain, "block fetch depth exceeded (%d > %d), discarding block slot=%d block_root=0x%x",
			depth, MaxBlockFetchDepth, block.Slot, blockRoot)
		return
	}

	logger.Warn(logger.Chain, "block parent missing slot=%d block_root=0x%x parent_root=0x%x depth=%d, storing as pending",
		block.Slot, blockRoot, parentRoot, depth)

	e.Pending.SetDepth(blockRoot, depth)
	e.Pending.SetSlot(blockRoot, block.Slot)
	missingRoot := e.Pending.ResolveAncestor(parentRoot)
	e.Pending.SetParent(blockRoot, parentRoot)
	e.Store.StorePendingBlock(blockRoot, signedBlock)
	e.Pending.AddChild(parentRoot, blockRoot)

	missingRoot, outcome := e.queueStoredAncestor(missingRoot, queue)
	switch outcome {
	case ancestorQueued:
	case ancestorMissing:
		e.queueMissingBlockFetch(missingRoot)
	case ancestorBelowFinalized:
		// The branch leaves the chain below finalization, so it can never be
		// canonical: drop it whole instead of fetching toward genesis.
		logger.Warn(logger.Chain, "discarding pending branch below the finalized slot: block slot=%d block_root=0x%x ancestor=0x%x",
			block.Slot, blockRoot, missingRoot)
		e.Pending.DiscardSubtree(missingRoot)
	case ancestorTooDeep:
		logger.Warn(logger.Chain, "stored ancestor walk exceeded %d blocks, discarding pending branch: block slot=%d block_root=0x%x",
			MaxBlockFetchDepth, block.Slot, blockRoot)
		e.Pending.DiscardSubtree(missingRoot)
	}
}

// evictFarthestPending frees a slot in the full pending buffer for a block at
// incomingSlot by discarding the entry farthest from the known chain. A full
// buffer must not reject blocks near the connect frontier: those are the only
// ones that can turn into imports, and turning them away is how a lagging
// node's buffer stays poisoned with an unconnectable tail forever. When the
// incoming block sits no closer than everything already held, it is the one
// that loses.
func (e *Engine) evictFarthestPending(incomingSlot uint64) bool {
	evictRoot, evictSlot, ok := e.Pending.HighestSlotEntry()
	if !ok || evictSlot <= incomingSlot {
		return false
	}
	logger.Warn(logger.Chain, "pending block cache full (%d), evicting slot=%d block_root=0x%x to admit slot=%d",
		MaxPendingBlocks, evictSlot, evictRoot, incomingSlot)
	e.Pending.DiscardSubtree(evictRoot)
	return true
}

type ancestorOutcome int

const (
	// ancestorQueued: a stored block whose parent has a state was queued for import.
	ancestorQueued ancestorOutcome = iota
	// ancestorMissing: the returned root has no stored header and must be fetched.
	ancestorMissing
	// ancestorBelowFinalized: the walk reached a stored block at or below the
	// finalized slot without meeting a state, so the branch split from the chain
	// before finalization. The returned root heads everything the walk linked.
	ancestorBelowFinalized
	// ancestorTooDeep: the walk passed MaxBlockFetchDepth stored blocks. A gap
	// that deep is range sync's to close, not a by-root walk's.
	ancestorTooDeep
)

// queueStoredAncestor walks back from missingRoot through blocks whose headers
// are stored but whose states are not, linking each into the pending buffer, until
// it finds one whose parent has a state and queues it for import.
//
// The walk ends at the finalized slot. Below it, canonical blocks keep their
// headers while their states are pruned, so a branch that split off before
// finalization would otherwise be walked header by header all the way to genesis
// on the dispatch loop, and every step would be linked into the buffer.
func (e *Engine) queueStoredAncestor(missingRoot [32]byte, queue *[]*types.SignedBlock) ([32]byte, ancestorOutcome) {
	finalizedSlot := e.Store.LatestFinalized().Slot
	for steps := 0; ; steps++ {
		header := e.Store.GetBlockHeader(missingRoot)
		if header == nil {
			return missingRoot, ancestorMissing
		}
		if header.Slot <= finalizedSlot {
			return missingRoot, ancestorBelowFinalized
		}
		if e.Store.HasState(header.ParentRoot) {
			if storedBlock := e.Store.GetSignedBlock(missingRoot); storedBlock != nil {
				*queue = append(*queue, storedBlock)
			}
			return missingRoot, ancestorQueued
		}
		if steps >= MaxBlockFetchDepth {
			return missingRoot, ancestorTooDeep
		}
		e.Pending.SetSlot(missingRoot, header.Slot)
		e.Pending.AddChild(header.ParentRoot, missingRoot)
		e.Pending.SetParent(missingRoot, header.ParentRoot)
		missingRoot = header.ParentRoot
	}
}

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

func (e *Engine) discardFinalizedPending(finalizedSlot uint64) {
	if discarded := e.Pending.DiscardAtOrBelow(finalizedSlot); discarded > 0 {
		logger.Info(logger.Store, "discarded %d finalized pending blocks (finalized_slot=%d)", discarded, finalizedSlot)
	}

	if removed := e.PendingAttestations.PruneBelow(finalizedSlot); removed > 0 {
		logger.Info(logger.Store, "discarded %d finalized pending attestations (finalized_slot=%d)", removed, finalizedSlot)
	}
}

func (e *Engine) onFailedRoot(failedRoot [32]byte) {
	// The fetch for this root is exhausted; drop its in-flight marker so it can be
	// re-requested if a later child reintroduces the gap.
	delete(e.fetchInFlight, failedRoot)

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
