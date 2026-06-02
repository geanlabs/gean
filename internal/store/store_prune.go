package store

import (
	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/storage"
)

// Pruning constants.
const (
	PruningIntervalSlots = 7200 // Periodic pruning every ~8 hours at 4s/slot
)

// PruneOnFinalization removes non-canonical data and stale attestation data
// after finalization advances.
func PruneOnFinalization(s *ConsensusStore, fc *forkchoice.ForkChoice, oldFinalizedSlot, newFinalizedSlot uint64, newFinalizedRoot [32]byte) {
	if newFinalizedSlot <= oldFinalizedSlot {
		return
	}

	canonical, nonCanonical := fc.GetCanonicalAnalysis(newFinalizedRoot)

	prunedStates := pruneStatesByRoots(s, nonCanonical)
	prunedBlocks := pruneBlocksByRoots(s, nonCanonical)

	// canonical[0] is the finalized root — keep it, prune earlier ancestors.
	if len(canonical) > 1 {
		prunedStates += pruneStatesByRoots(s, canonical[1:])
	}

	prunedChain := pruneLiveChain(s, newFinalizedSlot)

	prunedSigs := s.AttestationSignatures.PruneBelow(newFinalizedSlot)
	prunedKnown := s.KnownPayloads.PruneBelow(newFinalizedSlot)
	prunedNew := s.NewPayloads.PruneBelow(newFinalizedSlot)

	logger.Info(logger.Store, "pruning: finalized_slot=%d states=%d blocks=%d live_chain=%d gossip_sigs=%d payloads=%d non_canonical=%d",
		newFinalizedSlot, prunedStates, prunedBlocks, prunedChain, prunedSigs,
		prunedKnown+prunedNew, len(nonCanonical))
}

// PeriodicPrune runs canonicality-based pruning when finalization stalls.
func PeriodicPrune(s *ConsensusStore, fc *forkchoice.ForkChoice, currentSlot, finalizedSlot uint64) {
	if currentSlot == 0 || currentSlot%PruningIntervalSlots != 0 {
		return
	}

	// Only prune if finalization is stalled (2x interval behind).
	if finalizedSlot+2*PruningIntervalSlots >= currentSlot {
		return
	}

	logger.Warn(logger.Store, "finalization stalled: finalized_slot=%d current_slot=%d, running periodic pruning", finalizedSlot, currentSlot)

	// Get canonical ancestor at PruningIntervalSlots depth.
	ancestorRoot, ancestorSlot, ok := fc.GetCanonicalAncestorAtDepth(PruningIntervalSlots)
	if !ok || ancestorSlot <= finalizedSlot {
		return
	}

	// Prune non-canonical states below the ancestor.
	_, nonCanonical := fc.GetCanonicalAnalysis(ancestorRoot)
	prunedStates := pruneStatesByRoots(s, nonCanonical)
	prunedBlocks := pruneBlocksByRoots(s, nonCanonical)

	if prunedStates > 0 || prunedBlocks > 0 {
		logger.Info(logger.Store, "periodic pruning: ancestor_slot=%d states=%d blocks=%d non_canonical=%d",
			ancestorSlot, prunedStates, prunedBlocks, len(nonCanonical))
	}
}

// pruneStatesByRoots removes states for the given roots from DB.
func pruneStatesByRoots(s *ConsensusStore, roots [][32]byte) int {
	if len(roots) == 0 {
		return 0
	}

	keys := make([][]byte, len(roots))
	for i, root := range roots {
		k := make([]byte, 32)
		copy(k, root[:])
		keys[i] = k
	}

	wb, err := s.Backend.BeginWrite()
	if err != nil {
		return 0
	}
	wb.DeleteBatch(storage.TableStates, keys)
	wb.Commit()
	return len(roots)
}

// pruneBlocksByRoots removes block headers, bodies, and signatures for the given roots.
func pruneBlocksByRoots(s *ConsensusStore, roots [][32]byte) int {
	if len(roots) == 0 {
		return 0
	}

	keys := make([][]byte, len(roots))
	for i, root := range roots {
		k := make([]byte, 32)
		copy(k, root[:])
		keys[i] = k
	}

	wb, err := s.Backend.BeginWrite()
	if err != nil {
		return 0
	}
	wb.DeleteBatch(storage.TableBlockHeaders, keys)
	wb.DeleteBatch(storage.TableBlockBodies, keys)
	wb.DeleteBatch(storage.TableBlockSignatures, keys)
	wb.Commit()
	return len(roots)
}

// pruneLiveChain removes LiveChain entries with slot < finalizedSlot.
func pruneLiveChain(s *ConsensusStore, finalizedSlot uint64) int {
	rv, err := s.Backend.BeginRead()
	if err != nil {
		return 0
	}

	iter, err := rv.PrefixIterator(storage.TableLiveChain, nil)
	if err != nil {
		return 0
	}
	defer iter.Close()

	var keysToDelete [][]byte
	for iter.Next() {
		key := iter.Key()
		if len(key) < 8 {
			continue
		}
		slot, _ := storage.DecodeLiveChainKey(key)
		if slot < finalizedSlot {
			k := make([]byte, len(key))
			copy(k, key)
			keysToDelete = append(keysToDelete, k)
		}
	}

	if len(keysToDelete) == 0 {
		return 0
	}

	wb, err := s.Backend.BeginWrite()
	if err != nil {
		return 0
	}
	wb.DeleteBatch(storage.TableLiveChain, keysToDelete)
	wb.Commit()
	return len(keysToDelete)
}
