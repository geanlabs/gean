package store

import (
	"sync"

	"github.com/geanlabs/gean/internal/types"
)

// blockIndexCap bounds the in-memory block metadata during a finality stall,
// when finalization pruning stops trimming it: about six days of 4s slots, and
// eviction keeps the newest half. Eviction only costs speed: every lookup that
// misses falls back to the header table, so the index never has to be complete
// to be correct.
const blockIndexCap = 1 << 17

// blockMeta is the part of a block header that chain walks follow.
type blockMeta struct {
	slot   uint64
	parent [32]byte
}

// blockIndex keeps the walkable part of the chain in memory.
//
// The spec's fork choice walks parent links over store.blocks, an in-memory map,
// for the finalized checkpoint, for attestation ancestry checks and for the
// canonical chain. gean did the same walks over the header table on disk, one
// read per step, so their cost grew with the distance to finalization. During a
// long finality stall that distance grows by a block a slot.
//
// meta caches each header's slot and parent. Headers never change once written,
// so an entry cannot go stale; it only has to leave when its header is deleted.
// A miss reads the header table, so the cache can be partial or evicted freely.
//
// canonical maps each slot to the block the head's chain holds there, the zero
// root for an empty slot, over [canonicalLow, canonicalHeadSlot]. It is moved
// incrementally when the head changes, so a walk that reaches a canonical block
// can stop there and answer from it: below that block the chain is the canonical
// one. That bounds an ancestry check by how far its branch forked rather than by
// how far back its ancestor sits.
type blockIndex struct {
	mu   sync.RWMutex
	meta map[[32]byte]blockMeta

	canonical         map[uint64][32]byte
	canonicalValid    bool
	canonicalHead     [32]byte
	canonicalHeadSlot uint64
	canonicalLow      uint64
	// moving is set while moveCanonicalHead walks. Eviction waits for it to end:
	// a trim in mid-walk would delete slots the walk has already written and
	// leave holes it never revisits.
	moving bool
	// floor is the lowest slot the canonical walk descends to: the finalized slot
	// as of the last prune, or the store's finalized slot if that is higher, as it
	// is right after a restart. Nothing below it can change, and the per-slot
	// paths do not ask about it.
	floor uint64
}

// blockMetaLocked returns a block's slot and parent, reading the header table on
// a miss. The caller holds bi.mu for writing.
func (s *ConsensusStore) blockMetaLocked(root [32]byte) (blockMeta, bool) {
	if m, ok := s.blocks.meta[root]; ok {
		return m, true
	}
	header := s.GetBlockHeader(root)
	if header == nil {
		return blockMeta{}, false
	}
	m := blockMeta{slot: header.Slot, parent: header.ParentRoot}
	s.insertBlockMetaLocked(root, m)
	return m, true
}

// BlockSlotAndParent returns a stored block's slot and parent root, from memory
// when the index holds it and from the header table otherwise.
func (s *ConsensusStore) BlockSlotAndParent(root [32]byte) (slot uint64, parent [32]byte, ok bool) {
	if s == nil {
		return 0, types.ZeroRoot, false
	}
	s.blocks.mu.RLock()
	m, hit := s.blocks.meta[root]
	s.blocks.mu.RUnlock()
	if hit {
		return m.slot, m.parent, true
	}

	s.blocks.mu.Lock()
	defer s.blocks.mu.Unlock()
	m, found := s.blockMetaLocked(root)
	return m.slot, m.parent, found
}

// NoteStoredHeader records a header just written outside PutBlockHeader, such
// as by the block-import batch.
func (s *ConsensusStore) NoteStoredHeader(root [32]byte, header *types.BlockHeader) {
	if s == nil || header == nil {
		return
	}
	s.blocks.mu.Lock()
	defer s.blocks.mu.Unlock()
	s.insertBlockMetaLocked(root, blockMeta{slot: header.Slot, parent: header.ParentRoot})
}

func (s *ConsensusStore) insertBlockMetaLocked(root [32]byte, m blockMeta) {
	if s.blocks.meta == nil {
		s.blocks.meta = make(map[[32]byte]blockMeta)
	}
	s.blocks.meta[root] = m
	if len(s.blocks.meta) > blockIndexCap && !s.blocks.moving {
		s.evictBlockIndexLocked()
	}
}

// evictBlockIndexLocked brings the metadata back under the cap. A stall long
// enough to fill it gets here; finalization pruning keeps the index far smaller
// otherwise.
//
// The cutoff is taken from the canonical head, which only verified blocks move.
// Pending headers are stored before verification and may carry any slot, so a
// cutoff taken from the highest slot held would let one far-future header wipe
// the index. Forks or unverified headers can still hold it over the cap after
// the trim; the metadata is only a cache, so it is then dropped outright rather
// than rescanned on every insert.
func (s *ConsensusStore) evictBlockIndexLocked() {
	bi := &s.blocks
	if bi.canonicalValid && bi.canonicalHeadSlot > blockIndexCap/2 {
		s.trimBlockIndexLocked(bi.canonicalHeadSlot - blockIndexCap/2)
	}
	if len(bi.meta) > blockIndexCap*3/4 {
		clear(bi.meta)
	}
}

// trimBlockIndexLocked forgets metadata and canonical slots below cutoff.
func (s *ConsensusStore) trimBlockIndexLocked(cutoff uint64) {
	for root, m := range s.blocks.meta {
		if m.slot < cutoff {
			delete(s.blocks.meta, root)
		}
	}
	if !s.blocks.canonicalValid || cutoff <= s.blocks.canonicalLow {
		return
	}
	for slot := range s.blocks.canonical {
		if slot < cutoff {
			delete(s.blocks.canonical, slot)
		}
	}
	s.blocks.canonicalLow = cutoff
	if cutoff > s.blocks.canonicalHeadSlot {
		s.blocks.canonicalValid = false
	}
}

// forgetBlockHeaders drops the metadata of headers that have been deleted, so a
// walk sees them missing exactly as a read of the header table would. Deleting a
// canonical block invalidates the canonical map until the next head update
// rebuilds it.
func (s *ConsensusStore) forgetBlockHeaders(roots [][32]byte) {
	s.blocks.mu.Lock()
	defer s.blocks.mu.Unlock()
	for _, root := range roots {
		m, ok := s.blocks.meta[root]
		delete(s.blocks.meta, root)
		if ok && s.blocks.canonicalValid && s.blocks.canonical[m.slot] == root {
			s.blocks.canonicalValid = false
		}
	}
	if !s.blocks.canonicalValid {
		return
	}
	// A deleted header may not have been in memory; check the canonical map too.
	deleted := make(map[[32]byte]bool, len(roots))
	for _, root := range roots {
		deleted[root] = true
	}
	for _, root := range s.blocks.canonical {
		if deleted[root] {
			s.blocks.canonicalValid = false
			return
		}
	}
}

// pruneBlockIndexBelow moves the index floor up to a new finalized slot and
// forgets what lies below it.
func (s *ConsensusStore) pruneBlockIndexBelow(finalizedSlot uint64) {
	s.blocks.mu.Lock()
	defer s.blocks.mu.Unlock()
	if finalizedSlot <= s.blocks.floor {
		return
	}
	s.blocks.floor = finalizedSlot
	s.trimBlockIndexLocked(finalizedSlot)
}

// moveCanonicalHead points the canonical map at a new head. It walks back from the
// head, writing each slot, until it meets a block the map already holds at that
// slot: below a shared block the two chains agree. A head that extends the old
// one costs one step; a reorg costs its depth. Only the first update after start,
// or after the map was invalidated, walks down to the floor.
func (s *ConsensusStore) moveCanonicalHead(head [32]byte) {
	s.blocks.mu.Lock()
	defer s.blocks.mu.Unlock()
	bi := &s.blocks
	bi.moving = true
	defer func() {
		bi.moving = false
		if len(bi.meta) > blockIndexCap {
			s.evictBlockIndexLocked()
		}
	}()

	headMeta, ok := s.blockMetaLocked(head)
	if !ok {
		bi.canonicalValid = false
		return
	}
	floor := bi.floor
	if finalized := s.LatestFinalized(); finalized != nil && finalized.Slot > floor {
		floor = finalized.Slot
	}
	if !bi.canonicalValid || bi.canonical == nil {
		bi.canonical = make(map[uint64][32]byte)
		bi.canonicalValid = false
	}
	// Slots above the new head belong to the chain the head left.
	if bi.canonicalValid {
		for slot := headMeta.slot + 1; slot <= bi.canonicalHeadSlot; slot++ {
			delete(bi.canonical, slot)
		}
	}

	cur, curMeta := head, headMeta
	childSlot := headMeta.slot + 1
	joined := false
	low := headMeta.slot
	for {
		for slot := curMeta.slot + 1; slot < childSlot; slot++ {
			bi.canonical[slot] = types.ZeroRoot
		}
		if bi.canonicalValid && curMeta.slot >= bi.canonicalLow {
			if held, ok := bi.canonical[curMeta.slot]; ok && held == cur {
				joined = true
				break
			}
		}
		bi.canonical[curMeta.slot] = cur
		low = curMeta.slot
		if curMeta.slot <= floor || curMeta.slot == 0 {
			break
		}
		parentMeta, ok := s.blockMetaLocked(curMeta.parent)
		// A parent must sit at a lower slot; the transition rejects anything else.
		// The check keeps a corrupt header from looping this walk under the lock.
		if !ok || parentMeta.slot >= curMeta.slot {
			break
		}
		childSlot = curMeta.slot
		cur, curMeta = curMeta.parent, parentMeta
	}

	if !joined {
		bi.canonicalLow = low
		// A fresh walk may stop above slots a previous map still held.
		for slot := range bi.canonical {
			if slot < low {
				delete(bi.canonical, slot)
			}
		}
	}
	bi.canonicalHead = head
	bi.canonicalHeadSlot = headMeta.slot
	bi.canonicalValid = true
}

// CanonicalAncestorAt answers, in one consistent view, whether block is the
// canonical block at its slot and, if so, which block the canonical chain holds
// at ancestorSlot. The zero root means that slot is empty. ok is false when the
// map cannot say, and the caller must walk instead.
func (s *ConsensusStore) CanonicalAncestorAt(block [32]byte, blockSlot, ancestorSlot uint64) (ancestor [32]byte, ok bool) {
	if s == nil {
		return types.ZeroRoot, false
	}
	s.blocks.mu.RLock()
	defer s.blocks.mu.RUnlock()
	bi := &s.blocks
	if !bi.canonicalValid || ancestorSlot < bi.canonicalLow || blockSlot > bi.canonicalHeadSlot || ancestorSlot > blockSlot {
		return types.ZeroRoot, false
	}
	if held, found := bi.canonical[blockSlot]; !found || held != block {
		return types.ZeroRoot, false
	}
	held, found := bi.canonical[ancestorSlot]
	if !found {
		return types.ZeroRoot, false
	}
	return held, true
}

// canonicalRootsInRange returns the canonical block roots in [startSlot, endSlot)
// in ascending slot order, and whether the canonical map covers the request the
// way a walk from the head would: the walk is complete when it reaches a block
// below startSlot, or genesis.
func (s *ConsensusStore) canonicalRootsInRange(startSlot, endSlot uint64) ([][32]byte, bool) {
	s.blocks.mu.RLock()
	defer s.blocks.mu.RUnlock()
	bi := &s.blocks
	if !bi.canonicalValid || (bi.canonicalLow >= startSlot && bi.canonicalLow != 0) {
		return nil, false
	}
	var roots [][32]byte
	for slot := startSlot; slot < endSlot && slot <= bi.canonicalHeadSlot; slot++ {
		root, held := bi.canonical[slot]
		if !held {
			// Every slot in the covered window is written, empty ones as the
			// zero root. A gap means the map is not what it claims; the walk
			// answers instead of reading the gap as an empty slot.
			return nil, false
		}
		if !types.IsZeroRoot(root) {
			roots = append(roots, root)
		}
	}
	return roots, true
}
