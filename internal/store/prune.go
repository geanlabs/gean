package store

import (
	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/types"
)

const (
	PruningIntervalSlots = 7200

	// AttestationRetentionSlots is how far below the head the attestation-keyed
	// pools are kept once finalization stops advancing. Roughly an hour at
	// 4-second slots: long enough that a brief finality gap costs nothing, short
	// enough that a sustained stall does not grow the pools without limit.
	AttestationRetentionSlots = 1024

	// StateRetentionSlots is how far below the head post-states are kept once
	// finalization stops advancing: twice the attestation retention, so every
	// vote still kept can find its target state. A block on a fork older than
	// this still imports; the import path re-imports stored ancestors from the
	// nearest kept state.
	StateRetentionSlots = 2 * AttestationRetentionSlots
)

func PruneOnFinalization(s *ConsensusStore, fc *forkchoice.ForkChoice, oldFinalizedSlot, newFinalizedSlot uint64, newFinalizedRoot [32]byte) {
	if s == nil || s.Backend == nil || fc == nil {
		return
	}
	if newFinalizedSlot <= oldFinalizedSlot {
		return
	}

	canonical, nonCanonical := fc.GetCanonicalAnalysis(newFinalizedRoot)

	prunedStates := pruneStatesByRoots(s, nonCanonical)
	prunedBlocks := pruneBlocksByRoots(s, nonCanonical)

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

// PruneStaleAttestationPools clears the attestation-keyed pools against a
// head-relative cutoff instead of the finalized slot.
//
// PruneOnFinalization is the only other path that touches these three pools, and
// it runs on finalization alone. PeriodicPrune, the existing stall fallback,
// prunes non-canonical states and blocks and none of the pools, needs
// finalization to be more than two pruning intervals behind, and fires only on
// exact multiples of that interval. So the one situation that makes the pools
// grow without limit is also the one that switches off everything that empties
// them.
//
// Below the finalized slot this is a no-op: PruneOnFinalization already covers
// that range, and a cutoff at or under it would only repeat work.
func PruneStaleAttestationPools(s *ConsensusStore, headSlot, finalizedSlot uint64) {
	if s == nil || headSlot <= AttestationRetentionSlots {
		return
	}
	cutoff := headSlot - AttestationRetentionSlots
	if cutoff <= finalizedSlot {
		return
	}

	// All three pools key on the same target slot, so a data root leaves them
	// together and the order here does not matter.
	prunedSigs := s.AttestationSignatures.PruneStaleBelow(cutoff)
	prunedKnown := s.KnownPayloads.PruneStaleBelow(cutoff)
	prunedNew := s.NewPayloads.PruneStaleBelow(cutoff)

	if prunedSigs+prunedKnown+prunedNew == 0 {
		return
	}
	logger.Warn(logger.Store, "finalization lagging, pruned stale attestation pools: cutoff=%d head=%d finalized=%d gossip_sigs=%d payloads=%d",
		cutoff, headSlot, finalizedSlot, prunedSigs, prunedKnown+prunedNew)
}

// PruneStaleStates bounds the state table while finalization stalls, by
// deleting the post-states of imported blocks more than StateRetentionSlots
// below the head. PruneOnFinalization only removes states below the finalized
// slot, and PeriodicPrune only non-canonical ones, so without this every state
// since the last finalization stays on disk. The justified state is kept.
//
// The live chain is slot-ordered and states below statesPrunedBelow are already
// gone, unless import rebuilt one and lowered the mark, so each call only
// touches the newly aged slots. The mark is in memory: after
// a restart the first call repeats deletes, which are no-ops.
func PruneStaleStates(s *ConsensusStore, headSlot uint64, finalized, justified *types.Checkpoint) int {
	if s == nil || finalized == nil || headSlot <= StateRetentionSlots {
		return 0
	}
	cutoff := headSlot - StateRetentionSlots
	from := max(s.statesPrunedBelow, finalized.Slot+1)
	if cutoff <= from {
		return 0
	}

	entries, err := s.LiveChainRange(from, cutoff)
	if err != nil {
		logger.Error(logger.Store, "prune stale states: %v", err)
		return 0
	}
	var roots [][32]byte
	for _, e := range entries {
		if justified != nil && e.Root == justified.Root {
			continue
		}
		roots = append(roots, e.Root)
	}
	pruned := pruneStatesByRoots(s, roots)
	if pruned != len(roots) {
		return pruned
	}
	s.statesPrunedBelow = cutoff
	if pruned > 0 {
		logger.Warn(logger.Store, "finalization lagging, pruned stale states: cutoff=%d head=%d finalized=%d states=%d",
			cutoff, headSlot, finalized.Slot, pruned)
	}
	return pruned
}

// ObserveImportedState lowers PruneStaleStates' mark when import writes a state
// below it, as rebuilding a pruned parent does, so the next sweep reaches that
// state again.
func (s *ConsensusStore) ObserveImportedState(slot uint64) {
	s.statesPrunedBelow = min(s.statesPrunedBelow, slot)
}

func PeriodicPrune(s *ConsensusStore, fc *forkchoice.ForkChoice, currentSlot, finalizedSlot uint64) {
	if s == nil || s.Backend == nil || fc == nil {
		return
	}
	if currentSlot == 0 || currentSlot%PruningIntervalSlots != 0 {
		return
	}

	if finalizedSlot+2*PruningIntervalSlots >= currentSlot {
		return
	}

	logger.Warn(logger.Store, "finalization stalled: finalized_slot=%d current_slot=%d, running periodic pruning", finalizedSlot, currentSlot)

	ancestorRoot, ancestorSlot, ok := fc.AncestorAtDepth(s.Head(), PruningIntervalSlots)
	if !ok || ancestorSlot <= finalizedSlot {
		return
	}

	_, nonCanonical := fc.GetCanonicalAnalysis(ancestorRoot)
	prunedStates := pruneStatesByRoots(s, nonCanonical)
	prunedBlocks := pruneBlocksByRoots(s, nonCanonical)

	if prunedStates > 0 || prunedBlocks > 0 {
		logger.Info(logger.Store, "periodic pruning: ancestor_slot=%d states=%d blocks=%d non_canonical=%d",
			ancestorSlot, prunedStates, prunedBlocks, len(nonCanonical))
	}
}

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

	wb, err := s.beginWrite("prune states")
	if err != nil {
		logger.Error(logger.Store, "%v", err)
		return 0
	}
	if err := wb.DeleteBatch(storage.TableStates, keys); err != nil {
		logger.Error(logger.Store, "prune states: delete failed: %v", err)
		return 0
	}
	if !commitDeletes(wb, "prune states") {
		return 0
	}
	return len(roots)
}

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

	wb, err := s.beginWrite("prune blocks")
	if err != nil {
		logger.Error(logger.Store, "%v", err)
		return 0
	}
	if err := wb.DeleteBatch(storage.TableBlockHeaders, keys); err != nil {
		logger.Error(logger.Store, "prune blocks: delete headers failed: %v", err)
		return 0
	}
	if err := wb.DeleteBatch(storage.TableBlockBodies, keys); err != nil {
		logger.Error(logger.Store, "prune blocks: delete bodies failed: %v", err)
		return 0
	}
	if err := wb.DeleteBatch(storage.TableSignedBlocks, keys); err != nil {
		logger.Error(logger.Store, "prune blocks: delete signatures failed: %v", err)
		return 0
	}
	if !commitDeletes(wb, "prune blocks") {
		return 0
	}
	return len(roots)
}

func pruneLiveChain(s *ConsensusStore, finalizedSlot uint64) int {
	rv, err := s.beginRead("prune live chain")
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
		if len(key) < storage.LiveChainKeySize {
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

	wb, err := s.beginWrite("prune live chain")
	if err != nil {
		logger.Error(logger.Store, "%v", err)
		return 0
	}
	if err := wb.DeleteBatch(storage.TableLiveChain, keysToDelete); err != nil {
		logger.Error(logger.Store, "prune live chain: delete failed: %v", err)
		return 0
	}
	if !commitDeletes(wb, "prune live chain") {
		return 0
	}
	return len(keysToDelete)
}
