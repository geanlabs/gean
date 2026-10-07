package node

import (
	"context"

	"github.com/geanlabs/gean/blockprocessor"
	"github.com/geanlabs/gean/db"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/metrics"
	"github.com/geanlabs/gean/types"
)

func (e *Engine) onBlock(signedBlock *types.SignedBlock) {
	if signedBlock == nil || signedBlock.Block == nil {
		return
	}

	queue := []*types.SignedBlock{signedBlock}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		e.processOneBlock(current, &queue)
	}
}

func (e *Engine) processOneBlock(signedBlock *types.SignedBlock, queue *[]*types.SignedBlock) {
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
	delete(e.fetchInFlight, blockRoot)

	if e.store.HasState(blockRoot) {
		return
	}

	finalizedSlot := e.store.LatestFinalized().Slot
	if block.Slot < finalizedSlot {
		logger.Warn(logger.Chain, "rejecting pre-finalized block slot=%d block_root=0x%x finalized_slot=%d",
			block.Slot, blockRoot, finalizedSlot)
		return
	}

	hasParent := e.store.HasState(parentRoot)
	logger.Info(logger.Chain, "processing block slot=%d block_root=0x%x has_parent=%t", block.Slot, blockRoot, hasParent)

	if !hasParent {
		e.bufferMissingParentBlock(signedBlock, blockRoot, parentRoot, queue)
		return
	}

	e.importKnownParentBlock(signedBlock, blockRoot, parentRoot, queue)
}

func (e *Engine) importKnownParentBlock(
	signedBlock *types.SignedBlock,
	blockRoot [32]byte,
	parentRoot [32]byte,
	queue *[]*types.SignedBlock,
) {
	block := signedBlock.Block
	err := blockprocessor.OnBlock(e.store, e.scheme, signedBlock)
	if err != nil {
		logger.Error(logger.Chain, "block processing failed slot=%d block_root=0x%x: %v", block.Slot, blockRoot, err)
		return
	}
	e.dispatchRecovery(signedBlock)

	e.forkChoice.OnBlock(block.Slot, blockRoot, parentRoot)

	e.updateHead()
	e.pendingBlocks.ClearDepth(blockRoot)
	e.replayPendingAttestations(blockRoot)
	e.collectPendingChildren(blockRoot, queue)
}

// recordTableBytes refreshes the per-table storage-size gauge.
//
// This used to run on the import path, once per block. Both halves of that were
// wrong: the backend estimate was a full scan of every table (see
// pebbledb.Backend.EstimateTableBytes), and the import path is the dispatch
// goroutine, so the cost landed directly on the slot clock. A storage-size gauge
// is coarse by nature and does not need per-block resolution; it is sampled on
// its own goroutine now, like the gossip-mesh gauge.
func (e *Engine) recordTableBytes(ctx context.Context) {
	for _, table := range db.AllTables {
		// Bail between tables so a cancelled shutdown does not spend a whole
		// round on a database that is about to close. Responsiveness only:
		// Run waiting for its workers is what makes Close safe.
		if ctx.Err() != nil {
			return
		}
		metrics.SetTableBytes(string(table), e.store.EstimateTableBytes(table))
	}
}
