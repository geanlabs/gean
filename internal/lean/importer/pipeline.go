package importer

import (
	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/lean/types"
)

// Import takes one received block through the import pipeline:
//
//  1. skip blocks already held, or already imported below a pruned state;
//  2. reject blocks below the finalized slot (pipeline.go);
//  3. with the parent missing, hold the block and fetch the parent (pending.go, fetch.go);
//  4. with the parent known, validate, verify, run the state transition and
//     persist (process.go), then update fork choice and the head;
//  5. replay attestations waiting on the block (replay.go), and import the
//     children that were waiting on it, in the same call.
func (i *Importer) Import(signedBlock *types.SignedBlock) {
	if signedBlock == nil || signedBlock.Block == nil {
		return
	}
	i.importAttempts++

	var queue []*types.SignedBlock
	i.processOneBlock(signedBlock, &queue, false)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		i.processOneBlock(current, &queue, true)
	}
}

// processOneBlock imports or buffers one block. queued is true for blocks this
// import queued itself: stored ancestors rebuilding a pruned parent state, and
// pending children.
func (i *Importer) processOneBlock(signedBlock *types.SignedBlock, queue *[]*types.SignedBlock, queued bool) {
	if signedBlock == nil || signedBlock.Block == nil {
		return
	}

	block := signedBlock.Block
	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		logger.Error(logger.Chain, "block root failed slot=%d: %v", block.Slot, err)
		return
	}
	parentRoot := block.ParentRoot

	// We now hold this block (whether it imports or gets buffered), so any pending by-root
	// fetch for it is done — drop the in-flight marker so a future gap can re-request it.
	delete(i.fetchInFlight, blockRoot)

	if i.store.HasState(blockRoot) {
		return
	}
	// The live chain records every imported block, including those whose state
	// PruneStaleStates removed. Re-importing one received from outside would
	// rebuild every ancestor down to a kept state; only this import's own queue
	// rebuilds states.
	if !queued && i.store.HasLiveChainEntry(block.Slot, blockRoot) && i.store.HasBlockHeader(blockRoot) {
		return
	}

	// A block below the finalized slot with no state can never be canonical, and
	// nothing waiting on it can connect either.
	finalizedSlot := i.store.LatestFinalized().Slot
	if block.Slot < finalizedSlot {
		i.rejectPreFinalized(block.Slot, blockRoot, finalizedSlot)
		return
	}

	hasParent := i.store.HasState(parentRoot)
	logger.Info(logger.Chain, "processing block slot=%d block_root=0x%x has_parent=%t", block.Slot, blockRoot, hasParent)

	// With its parent missing, a block at the finalized slot is no better: it
	// can't be the finalized block, which has a state. The spec's head sync
	// drops parent-missing blocks at or below finalized before caching them.
	if !hasParent && block.Slot == finalizedSlot {
		i.rejectPreFinalized(block.Slot, blockRoot, finalizedSlot)
		return
	}

	if !hasParent {
		i.bufferMissingParentBlock(signedBlock, blockRoot, parentRoot, queue)
		return
	}

	i.importKnownParentBlock(signedBlock, blockRoot, parentRoot, queue)
}

func (i *Importer) rejectPreFinalized(slot uint64, blockRoot [32]byte, finalizedSlot uint64) {
	logger.Warn(logger.Chain, "rejecting pre-finalized block slot=%d block_root=0x%x finalized_slot=%d",
		slot, blockRoot, finalizedSlot)
	i.Pending.DiscardSubtree(blockRoot)
}

func (i *Importer) importKnownParentBlock(
	signedBlock *types.SignedBlock,
	blockRoot [32]byte,
	parentRoot [32]byte,
	queue *[]*types.SignedBlock,
) {
	block := signedBlock.Block
	err := OnBlock(i.store, signedBlock)
	if err != nil {
		logger.Error(logger.Chain, "block processing failed slot=%d block_root=0x%x: %v", block.Slot, blockRoot, err)
		return
	}
	if i.hooks.Imported != nil {
		i.hooks.Imported(signedBlock)
	}

	i.fc.OnBlock(block.Slot, blockRoot, parentRoot)

	if i.hooks.UpdateHead != nil {
		i.hooks.UpdateHead()
	}
	i.Pending.MarkImported(blockRoot)
	i.replayPendingAttestations(blockRoot)
	i.collectPendingChildren(blockRoot, queue)
}

// Drain imports the blocks already queued when the drain starts and no more.
// Sync delivery blocks on the queue, so a producer refills every slot the drain
// frees: draining until the queue reads empty hands the tick loop an unbounded
// amount of work, and store.OnTick has already run for this tick — the store
// clock then sits stale for the whole drain and correctly-timed blocks are
// rejected as beyond the future horizon. Whatever arrives mid-drain is picked up
// by the dispatch loop or the next tick.
func (i *Importer) Drain() int {
	drained := 0
drain:
	for remaining := len(i.blocks); remaining > 0; remaining-- {
		select {
		case block := <-i.blocks:
			i.Import(block)
			drained++
		default:
			// The dispatch loop took part of the batch first; nothing left.
			break drain
		}
	}
	if drained > 0 {
		logger.Info(logger.Chain, "drained %d pending blocks before attestation", drained)
	}
	return drained
}
