package store

import (
	"bytes"

	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/types"
)

const (
	PruningIntervalSlots = 7200

	// BlockProofRetentionSlots is how far below the head a finalized block keeps
	// its stored signed block, about a day at 4-second slots. Peers may only ask
	// for the last types.MinSlotsForBlockRequests slots, so an older proof is
	// never asked for, and a range that reaches one is refused rather than
	// served with a gap. The header and body stay in their own tables.
	BlockProofRetentionSlots = 21600

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

	// StateCheckpointSlots is the spacing of the ancestor states kept below
	// StateRetentionSlots during a stall: the most a rebuild for an old fork
	// point replays, and one kept state per this many slots of stall.
	StateCheckpointSlots = 64
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

	// Finalized history keeps its live-chain entry until the block is a day old,
	// then loses the entry and its stored signed block together.
	historyCutoff := uint64(0)
	if head := s.HeadSlot(); head > BlockProofRetentionSlots {
		historyCutoff = min(newFinalizedSlot, head-BlockProofRetentionSlots)
	}
	prunedChain := pruneFinalizedHistory(s, historyCutoff)

	prunedSigs := s.AttestationSignatures.PruneBelow(newFinalizedSlot)
	prunedKnown := s.KnownPayloads.PruneBelow(newFinalizedSlot)
	prunedNew := s.NewPayloads.PruneBelow(newFinalizedSlot)

	logger.Info(logger.Store, "pruning: finalized_slot=%d states=%d blocks=%d history=%d gossip_sigs=%d payloads=%d non_canonical=%d",
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
// deleting the post-states of the head's ancestors more than
// StateRetentionSlots below the head. PruneOnFinalization only removes states
// below the finalized slot, and PeriodicPrune only non-canonical ones, so
// without this every state since the last finalization stays on disk.
//
// Three kinds of state are kept:
//   - side branches: a branch being synced onto must keep its states, or each
//     sweep would delete what the last import built and the next block would
//     rebuild the whole branch;
//   - the justified state;
//   - the lowest ancestor in each StateCheckpointSlots bucket, so rebuilding
//     a pruned parent for a block on an old fork point replays at most that
//     many slots.
//
// Ancestors below statesPrunedBelow were pruned by an earlier call, so the walk
// stops at the first missing state below it; import lowers the mark when it
// rebuilds a state there. The mark is in memory: after a restart the first
// call checks every ancestor down to the finalized slot once.
func PruneStaleStates(s *ConsensusStore, fc *forkchoice.ForkChoice, headRoot [32]byte, headSlot uint64, finalized, justified *types.Checkpoint) int {
	if s == nil || fc == nil || finalized == nil || justified == nil || headSlot <= StateRetentionSlots {
		return 0
	}
	cutoff := headSlot - StateRetentionSlots
	if cutoff <= finalized.Slot+1 {
		return 0
	}
	// A justified state skipped by an earlier call sits below the mark once
	// justified moves on; lowering the mark lets this walk reach it.
	if justified.Slot != s.statesKeptJustifiedSlot {
		s.statesPrunedBelow = min(s.statesPrunedBelow, s.statesKeptJustifiedSlot)
		s.statesKeptJustifiedSlot = justified.Slot
	}

	var roots [][32]byte
	fc.WalkAncestors(headRoot, func(root [32]byte, slot, parentSlot uint64) bool {
		if slot >= cutoff {
			return true
		}
		if slot <= finalized.Slot {
			return false
		}
		if root == justified.Root || parentSlot/StateCheckpointSlots < slot/StateCheckpointSlots {
			return true
		}
		has := s.HasState(root)
		if has {
			roots = append(roots, root)
		}
		return has || slot >= s.statesPrunedBelow
	})
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

// pruneFinalizedHistory deletes, for every live-chain entry below cutoff, the
// entry and the block's stored signed block, which carries its proof. Headers
// and bodies stay. Entries are keyed by slot first, so the scan stops at the
// first one at or above cutoff and only touches what it deletes. Non-canonical
// blocks below finalization were already removed whole; their rows are gone and
// the deletes are no-ops.
func pruneFinalizedHistory(s *ConsensusStore, cutoff uint64) int {
	rv, err := s.beginRead("prune finalized history")
	if err != nil {
		return 0
	}

	iter, err := rv.PrefixIterator(storage.TableLiveChain, nil)
	if err != nil {
		return 0
	}
	defer iter.Close()

	var entries, roots [][]byte
	for iter.Next() {
		key := iter.Key()
		if len(key) < storage.LiveChainKeySize {
			continue
		}
		slot, root := storage.DecodeLiveChainKey(key)
		if slot >= cutoff {
			break
		}
		entries = append(entries, bytes.Clone(key))
		roots = append(roots, bytes.Clone(root[:]))
	}

	if len(entries) == 0 {
		return 0
	}

	wb, err := s.beginWrite("prune finalized history")
	if err != nil {
		logger.Error(logger.Store, "%v", err)
		return 0
	}
	if err := wb.DeleteBatch(storage.TableLiveChain, entries); err != nil {
		logger.Error(logger.Store, "prune finalized history: delete live chain: %v", err)
		return 0
	}
	if err := wb.DeleteBatch(storage.TableSignedBlocks, roots); err != nil {
		logger.Error(logger.Store, "prune finalized history: delete signed blocks: %v", err)
		return 0
	}
	if !commitDeletes(wb, "prune finalized history") {
		return 0
	}
	return len(entries)
}
