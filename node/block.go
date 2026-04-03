package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/xmss"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/types"
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
	// Canonicality-based pruning: prune non-canonical forks + old finalized ancestors.
	newFinalized := e.Store.LatestFinalized()
	if newFinalized.Slot > oldFinalizedSlot {
		PruneOnFinalization(e.Store, e.FC, oldFinalizedSlot, newFinalized.Slot, newFinalized.Root)
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

	// Check if parent state exists.
	if !e.Store.HasState(parentRoot) {
		logger.Warn(logger.Chain, "block parent missing slot=%d block_root=0x%x parent_root=0x%x, storing as pending",
			block.Slot, blockRoot, parentRoot)

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

		// Request the actual missing block from network.
		if e.P2P != nil {
			logger.Info(logger.Sync, "requesting missing block block_root=0x%x from network", missingRoot)
			go func(root [32]byte) {
				blocks, err := e.P2P.FetchBlocksByRootWithRetry(context.Background(), [][32]byte{root})
				if err != nil || len(blocks) == 0 {
					return
				}
				for _, result := range blocks {
					if result.Block != nil && len(result.Block) > 0 {
						for _, b := range result.Block {
							e.OnBlock(b)
						}
					}
				}
			}(missingRoot)
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

		childBlock := e.Store.GetSignedBlock(childRoot)
		if childBlock == nil {
			logger.Warn(logger.Chain, "pending block block_root=0x%x missing from DB, skipping", childRoot)
			continue
		}
		*queue = append(*queue, childBlock)
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
	pubkey := targetState.Validators[att.ValidatorID].Pubkey

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
