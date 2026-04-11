package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/p2p"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss"
)

// onBlock processes a received block using an iterative work queue.
func (e *Engine) onBlock(signedBlock *types.SignedBlockWithAttestation) {
	oldFinalizedSlot := e.Store.LatestFinalized().Slot

	queue := []*types.SignedBlockWithAttestation{signedBlock}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		e.processOneBlock(current, &queue)
	}

	// Prune AFTER the entire cascade completes — not mid-cascade.
	newFinalized := e.Store.LatestFinalized()
	if newFinalized.Slot > oldFinalizedSlot {
		PruneOnFinalization(e.Store, e.FC, oldFinalizedSlot, newFinalized.Slot, newFinalized.Root)
		e.discardFinalizedPending(newFinalized.Slot)
	}
}

func (e *Engine) processOneBlock(signedBlock *types.SignedBlockWithAttestation, queue *[]*types.SignedBlockWithAttestation) {
	block := signedBlock.Block.Block
	blockRoot, _ := block.HashTreeRoot()
	parentRoot := block.ParentRoot

	// Skip if already processed.
	if e.Store.HasState(blockRoot) {
		return
	}

	hasParent := e.Store.HasState(parentRoot)
	logger.Info(logger.Chain, "processing block slot=%d block_root=0x%x has_parent=%t", block.Slot, blockRoot, hasParent)

	// Check if parent state exists.
	if !hasParent {
		// Check pending block cache limit.
		if e.pendingBlockCount() >= MaxPendingBlocks {
			logger.Warn(logger.Chain, "pending block cache full (%d), rejecting block slot=%d block_root=0x%x",
				MaxPendingBlocks, block.Slot, blockRoot)
			return
		}

		// Compute depth: parent's depth + 1.
		depth := 1
		if parentDepth, ok := e.PendingBlockDepths[parentRoot]; ok {
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
		e.PendingBlockDepths[blockRoot] = depth

		// Resolve the actual missing ancestor by walking the chain.
		missingRoot := parentRoot
		for {
			ancestor, ok := e.PendingBlockParents[missingRoot]
			if !ok {
				break
			}
			missingRoot = ancestor
		}

		e.PendingBlockParents[blockRoot] = missingRoot

		// Store block in DB as pending (no LiveChain entry — invisible to fork choice).
		e.Store.StorePendingBlock(blockRoot, signedBlock)

		// Track parent→child relationship in memory.
		children, ok := e.PendingBlocks[parentRoot]
		if !ok {
			children = make(map[[32]byte]bool)
			e.PendingBlocks[parentRoot] = children
		}
		children[blockRoot] = true

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
			pChildren, ok := e.PendingBlocks[header.ParentRoot]
			if !ok {
				pChildren = make(map[[32]byte]bool)
				e.PendingBlocks[header.ParentRoot] = pChildren
			}
			pChildren[missingRoot] = true
			e.PendingBlockParents[missingRoot] = header.ParentRoot
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
	err := OnBlock(e.Store, signedBlock, e.Keys.ValidatorIDs())
	ObserveBlockProcessingTime(time.Since(blockStart).Seconds())
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

	// Update head BEFORE processing proposer attestation.
	e.updateHead(false)

	// Process proposer attestation.
	ProcessProposerAttestation(e.Store, signedBlock, true)

	// Clear depth tracking for this block (now processed).
	delete(e.PendingBlockDepths, blockRoot)

	// Cascade: enqueue pending children for processing.
	e.collectPendingChildren(blockRoot, queue)
}

// collectPendingChildren moves pending children of parent into the work queue.
func (e *Engine) collectPendingChildren(parentRoot [32]byte, queue *[]*types.SignedBlockWithAttestation) {
	childRoots, ok := e.PendingBlocks[parentRoot]
	if !ok {
		return
	}
	delete(e.PendingBlocks, parentRoot)

	logger.Info(logger.Chain, "processing %d pending children of parent_root=0x%x", len(childRoots), parentRoot)

	for childRoot := range childRoots {
		delete(e.PendingBlockParents, childRoot)
		delete(e.PendingBlockDepths, childRoot)

		childBlock := e.Store.GetSignedBlock(childRoot)
		if childBlock == nil {
			logger.Warn(logger.Chain, "pending block block_root=0x%x missing from DB, skipping", childRoot)
			continue
		}
		*queue = append(*queue, childBlock)
	}
}

// pendingBlockCount returns the total number of pending blocks across all parents.
func (e *Engine) pendingBlockCount() int {
	count := 0
	for _, children := range e.PendingBlocks {
		count += len(children)
	}
	return count
}

// discardFinalizedPending removes all pending blocks at or below the finalized slot.
// Their subtrees are also discarded since they can never be processed.
func (e *Engine) discardFinalizedPending(finalizedSlot uint64) {
	discarded := 0

	// Collect parent roots to discard.
	var parentsToDiscard [][32]byte
	for parentRoot, children := range e.PendingBlocks {
		for childRoot := range children {
			header := e.Store.GetBlockHeader(childRoot)
			if header != nil && header.Slot <= finalizedSlot {
				// This pending block is at/below finalized — discard entire subtree.
				e.discardPendingSubtree(childRoot)
				delete(children, childRoot)
				discarded++
			}
		}
		if len(children) == 0 {
			parentsToDiscard = append(parentsToDiscard, parentRoot)
		}
	}

	for _, parentRoot := range parentsToDiscard {
		delete(e.PendingBlocks, parentRoot)
	}

	if discarded > 0 {
		logger.Info(logger.Store, "discarded %d finalized pending blocks (finalized_slot=%d)", discarded, finalizedSlot)
	}
}

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
	children, ok := e.PendingBlocks[failedRoot]
	if !ok {
		return
	}
	delete(e.PendingBlocks, failedRoot)

	discarded := 0
	for childRoot := range children {
		e.discardPendingSubtree(childRoot)
		discarded++
	}
	logger.Warn(logger.Sync, "fetch exhausted for root 0x%x, discarded %d pending child block(s)", failedRoot, discarded)
}

// discardPendingSubtree recursively discards a pending block and all its descendants.
func (e *Engine) discardPendingSubtree(blockRoot [32]byte) {
	delete(e.PendingBlockParents, blockRoot)
	delete(e.PendingBlockDepths, blockRoot)

	children, ok := e.PendingBlocks[blockRoot]
	if !ok {
		return
	}
	delete(e.PendingBlocks, blockRoot)

	for childRoot := range children {
		e.discardPendingSubtree(childRoot)
	}
}

// onGossipAttestation validates and stores an individual attestation.
func (e *Engine) onGossipAttestation(att *types.SignedAttestation) {
	// Validate attestation data.
	if err := ValidateAttestationData(e.Store, att.Data); err != nil {
		return
	}

	// Get validator pubkey from target state.
	targetState := e.Store.GetState(att.Data.Target.Root)
	if targetState == nil {
		return
	}
	if att.ValidatorID >= uint64(len(targetState.Validators)) {
		return
	}
	pubkey := targetState.Validators[att.ValidatorID].AttestationPubkey

	// Verify XMSS signature.
	dataRoot, _ := att.Data.HashTreeRoot()
	slot := uint32(att.Data.Slot)

	IncPqSigAttestationSigsTotal()
	verifyStart := time.Now()
	valid, err := verifyAttestation(pubkey, slot, dataRoot, att.Signature)
	ObservePqSigVerificationTime(time.Since(verifyStart).Seconds())
	if err != nil || !valid {
		IncPqSigAttestationSigsInvalid()
		IncAttestationsInvalid()
		return
	}
	IncPqSigAttestationSigsValid()
	IncAttestationsValid(1)

	// Parse signature to opaque C handle for aggregation.
	sigHandle, parseErr := xmss.ParseSignature(att.Signature[:])

	// Store for aggregation.
	logger.Info(logger.Gossip, "attestation verified: validator=%d slot=%d dataRoot=%x", att.ValidatorID, att.Data.Slot, dataRoot)
	e.Store.GossipSignatures.InsertWithHandle(dataRoot, att.Data, att.ValidatorID, att.Signature, sigHandle, parseErr)
}

// onGossipAggregatedAttestation validates and stores an aggregated attestation.
func (e *Engine) onGossipAggregatedAttestation(agg *types.SignedAggregatedAttestation) {
	// Validate attestation data.
	if err := ValidateAttestationData(e.Store, agg.Data); err != nil {
		return
	}

	// Verify aggregated proof.
	if agg.Proof != nil && len(agg.Proof.ProofData) > 0 {
		targetState := e.Store.GetState(agg.Data.Target.Root)
		if targetState == nil {
			return
		}

		participantIDs := types.BitlistIndices(agg.Proof.Participants)
		if err := verifyAggregatedProof(targetState, participantIDs, agg.Data, agg.Proof.ProofData); err != nil {
			logger.Error(logger.Signature, "aggregated attestation verification failed: %v", err)
			return
		}
	}

	// Store in new payloads.
	dataRoot, _ := agg.Data.HashTreeRoot()
	e.Store.NewPayloads.Push(dataRoot, agg.Data, agg.Proof)
}
