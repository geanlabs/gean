package node

import (
	"github.com/geanlabs/gean/internal/blockprocessor"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func (e *Engine) onBlock(signedBlock *types.SignedBlock) {
	if signedBlock == nil || signedBlock.Block == nil {
		return
	}

	oldFinalizedSlot := e.Store.LatestFinalized().Slot
	queue := []*types.SignedBlock{signedBlock}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		e.processOneBlock(current, &queue)
	}

	newFinalized := e.Store.LatestFinalized()
	if newFinalized.Slot > oldFinalizedSlot {
		store.PruneOnFinalization(e.Store, e.FC, oldFinalizedSlot, newFinalized.Slot, newFinalized.Root)
		e.discardFinalizedPending(newFinalized.Slot)
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

	if e.Store.HasState(blockRoot) {
		return
	}

	finalizedSlot := e.Store.LatestFinalized().Slot
	if block.Slot < finalizedSlot {
		logger.Warn(logger.Chain, "rejecting pre-finalized block slot=%d block_root=0x%x finalized_slot=%d",
			block.Slot, blockRoot, finalizedSlot)
		return
	}

	hasParent := e.Store.HasState(parentRoot)
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
	err := blockprocessor.OnBlock(e.Store, signedBlock)
	if err != nil {
		logger.Error(logger.Chain, "block processing failed slot=%d block_root=0x%x: %v", block.Slot, blockRoot, err)
		return
	}

	e.FC.OnBlock(block.Slot, blockRoot, parentRoot)

	finalized := e.Store.LatestFinalized()
	if finalized.Slot > 0 {
		e.FC.Prune(finalized.Root)
	}

	e.updateHead()
	e.Pending.ClearDepth(blockRoot)
	e.replayPendingAttestations(blockRoot)
	e.collectPendingChildren(blockRoot, queue)
}
