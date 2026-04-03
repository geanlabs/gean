package node

import (
	"sort"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/storage"
)

// PruneOnFinalization performs lightweight pruning when finalization advances.
// Prunes live chain entries and gossip signatures below the finalized slot.
func PruneOnFinalization(s *ConsensusStore, finalizedSlot uint64) {
	prunedChain := pruneLiveChain(s, finalizedSlot)
	prunedSigs := s.GossipSignatures.PruneBelow(finalizedSlot)

	if prunedChain > 0 || prunedSigs > 0 {
		logger.Info(logger.Store, "finalization pruning: live_chain=%d gossip_sigs=%d finalized_slot=%d",
			prunedChain, prunedSigs, finalizedSlot)
	}
}

// PruneOldData performs heavy pruning of old states and blocks.
// Must be called AFTER a block cascade completes — not mid-cascade.
func PruneOldData(s *ConsensusStore) {
	protectedRoots := map[[32]byte]bool{
		s.LatestFinalized().Root: true,
		s.LatestJustified().Root: true,
	}

	prunedStates := pruneOldEntries(s, storage.TableStates, storage.StatesToKeep, protectedRoots)
	prunedBlocks := pruneOldBlocks(s, storage.BlocksToKeep, protectedRoots)

	if prunedStates > 0 || prunedBlocks > 0 {
		logger.Info(logger.Store, "pruned old data: states=%d blocks=%d", prunedStates, prunedBlocks)
	}
}

// pruneLiveChain removes LiveChain entries with slot < finalizedSlot.
// LiveChain keys are big-endian slot || root, so lexicographic scan works.
func pruneLiveChain(s *ConsensusStore, finalizedSlot uint64) int {
	rv, err := s.Backend.BeginRead()
	if err != nil {
		return 0
	}

	// Scan all live chain entries to find ones below finalized.
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

// rootSlotEntry pairs a root with its slot for sorting.
type rootSlotEntry struct {
	root [32]byte
	slot uint64
}

// pruneOldEntries prunes entries from a root-keyed table (States or BlockHeaders)
// that exceed the retention window, protecting specified roots.
func pruneOldEntries(s *ConsensusStore, table storage.Table, keepCount int, protected map[[32]byte]bool) int {
	// Collect all entries with their slots by cross-referencing block headers.
	rv, err := s.Backend.BeginRead()
	if err != nil {
		return 0
	}

	iter, err := rv.PrefixIterator(table, nil)
	if err != nil {
		return 0
	}

	var entries []rootSlotEntry
	for iter.Next() {
		key := iter.Key()
		if len(key) != 32 {
			continue
		}
		var root [32]byte
		copy(root[:], key)

		// Look up slot from block header.
		headerBytes, _ := rv.Get(storage.TableBlockHeaders, key)
		if headerBytes == nil {
			continue
		}
		// Slot is the first 8 bytes of the BlockHeader SSZ (uint64 LE).
		if len(headerBytes) < 8 {
			continue
		}
		slot := uint64(0)
		for i := 0; i < 8; i++ {
			slot |= uint64(headerBytes[i]) << (i * 8)
		}

		entries = append(entries, rootSlotEntry{root: root, slot: slot})
	}
	iter.Close()

	if len(entries) <= keepCount {
		return 0
	}

	// Sort by slot descending (newest first).
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].slot > entries[j].slot
	})

	// Skip the newest keepCount, prune the rest (unless protected).
	var keysToDelete [][]byte
	for _, e := range entries[keepCount:] {
		if protected[e.root] {
			continue
		}
		k := make([]byte, 32)
		copy(k, e.root[:])
		keysToDelete = append(keysToDelete, k)
	}

	if len(keysToDelete) == 0 {
		return 0
	}

	wb, err := s.Backend.BeginWrite()
	if err != nil {
		return 0
	}
	wb.DeleteBatch(table, keysToDelete)
	wb.Commit()
	return len(keysToDelete)
}

// pruneOldBlocks prunes old block headers, bodies, and signatures atomically.
func pruneOldBlocks(s *ConsensusStore, keepCount int, protected map[[32]byte]bool) int {
	rv, err := s.Backend.BeginRead()
	if err != nil {
		return 0
	}

	iter, err := rv.PrefixIterator(storage.TableBlockHeaders, nil)
	if err != nil {
		return 0
	}

	var entries []rootSlotEntry
	for iter.Next() {
		key := iter.Key()
		if len(key) != 32 {
			continue
		}
		var root [32]byte
		copy(root[:], key)

		val := iter.Value()
		if len(val) < 8 {
			continue
		}
		slot := uint64(0)
		for i := 0; i < 8; i++ {
			slot |= uint64(val[i]) << (i * 8)
		}

		entries = append(entries, rootSlotEntry{root: root, slot: slot})
	}
	iter.Close()

	if len(entries) <= keepCount {
		return 0
	}

	// Sort by slot descending (newest first).
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].slot > entries[j].slot
	})

	// Skip the newest keepCount, prune the rest (unless protected).
	var keysToDelete [][]byte
	for _, e := range entries[keepCount:] {
		if protected[e.root] {
			continue
		}
		k := make([]byte, 32)
		copy(k, e.root[:])
		keysToDelete = append(keysToDelete, k)
	}

	if len(keysToDelete) == 0 {
		return 0
	}

	// Delete from all 3 block tables atomically.
	wb, err := s.Backend.BeginWrite()
	if err != nil {
		return 0
	}
	wb.DeleteBatch(storage.TableBlockHeaders, keysToDelete)
	wb.DeleteBatch(storage.TableBlockBodies, keysToDelete)
	wb.DeleteBatch(storage.TableBlockSignatures, keysToDelete)
	wb.Commit()
	return len(keysToDelete)
}
