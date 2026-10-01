package store

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/types"
)

// headerWalkRange is GetCanonicalBlocksInRange as it was before the block index:
// a walk back from the head, one header read per step.
func headerWalkRange(s *ConsensusStore, startSlot, count uint64) ([][32]byte, bool) {
	if count == 0 || startSlot > ^uint64(0)-count {
		return nil, false
	}
	endSlot := startSlot + count
	var roots [][32]byte
	complete := false
	root := s.Head()
	for {
		header := s.GetBlockHeader(root)
		if header == nil {
			break
		}
		if header.Slot < startSlot {
			complete = true
			break
		}
		if header.Slot < endSlot && s.GetSignedBlock(root) != nil {
			roots = append(roots, root)
		}
		if header.Slot == 0 {
			complete = true
			break
		}
		root = header.ParentRoot
	}
	for i, j := 0, len(roots)-1; i < j; i, j = i+1, j-1 {
		roots[i], roots[j] = roots[j], roots[i]
	}
	return roots, complete
}

// headerClimb is the ancestry climb over the header table, as reference.
func headerClimb(s *ConsensusStore, ancestor [32]byte, ancestorSlot uint64, descendant [32]byte) bool {
	current := descendant
	for {
		header := s.GetBlockHeader(current)
		if header == nil || header.Slot < ancestorSlot {
			return false
		}
		if header.Slot == ancestorSlot {
			return current == ancestor
		}
		current = header.ParentRoot
	}
}

// indexedClimb is the climb as checkpointIsAncestor does it, with the canonical
// shortcut, so the store package can check the index without importing it.
func indexedClimb(s *ConsensusStore, ancestor [32]byte, ancestorSlot uint64, descendant [32]byte) bool {
	current := descendant
	for {
		slot, parent, ok := s.BlockSlotAndParent(current)
		if !ok || slot < ancestorSlot {
			return false
		}
		if slot == ancestorSlot {
			return current == ancestor
		}
		if held, ok := s.CanonicalAncestorAt(current, slot, ancestorSlot); ok {
			return !types.IsZeroRoot(held) && held == ancestor
		}
		current = parent
	}
}

type indexTestBlock struct {
	root [32]byte
	slot uint64
}

func storeIndexTestBlock(t *testing.T, s *ConsensusStore, root, parent [32]byte, slot uint64) {
	t.Helper()
	err := s.StorePendingBlock(root, &types.SignedBlock{
		Block: &types.Block{Slot: slot, ParentRoot: parent, Body: &types.BlockBody{}},
		Proof: &types.MultiMessageAggregate{},
	})
	if err != nil {
		t.Fatalf("store block: %v", err)
	}
}

func randomIndexChain(t *testing.T, rng *rand.Rand, s *ConsensusStore, n int) []indexTestBlock {
	t.Helper()
	genesis := indexTestBlock{root: [32]byte{0x01}, slot: 0}
	storeIndexTestBlock(t, s, genesis.root, types.ZeroRoot, 0)
	s.SetLatestFinalized(&types.Checkpoint{Root: genesis.root})
	blocks := []indexTestBlock{genesis}
	for i := 0; i < n; i++ {
		parent := blocks[len(blocks)-1-rng.Intn(min(len(blocks), 5))]
		child := indexTestBlock{root: [32]byte{0x02, byte(i), byte(i >> 8)}, slot: parent.slot + 1 + uint64(rng.Intn(3))}
		storeIndexTestBlock(t, s, child.root, parent.root, child.slot)
		blocks = append(blocks, child)
		if rng.Intn(3) > 0 {
			s.SetHead(blocks[len(blocks)-1-rng.Intn(min(len(blocks), 8))].root)
		}
	}
	return blocks
}

func assertIndexMatchesHeaders(t *testing.T, rng *rand.Rand, s *ConsensusStore, blocks []indexTestBlock, label string) {
	t.Helper()
	for range 200 {
		a, d := blocks[rng.Intn(len(blocks))], blocks[rng.Intn(len(blocks))]
		if got, want := indexedClimb(s, a.root, a.slot, d.root), headerClimb(s, a.root, a.slot, d.root); got != want {
			t.Fatalf("%s: ancestor(%d->%d) = %t, climb gives %t", label, a.slot, d.slot, got, want)
		}
	}
	maxSlot := blocks[len(blocks)-1].slot + 3
	for range 100 {
		start, count := uint64(rng.Intn(int(maxSlot))), uint64(1+rng.Intn(40))
		blocks, complete := s.GetCanonicalBlocksInRange(start, count)
		wantRoots, wantComplete := headerWalkRange(s, start, count)
		if complete != wantComplete || len(blocks) != len(wantRoots) {
			t.Fatalf("%s: range [%d,+%d) = %d blocks complete=%t, walk gives %d complete=%t",
				label, start, count, len(blocks), complete, len(wantRoots), wantComplete)
		}
		for i, b := range blocks {
			if b.Block.Slot != s.GetBlockHeader(wantRoots[i]).Slot {
				t.Fatalf("%s: range [%d,+%d) block %d at slot %d, walk gives root at slot %d",
					label, start, count, i, b.Block.Slot, s.GetBlockHeader(wantRoots[i]).Slot)
			}
		}
	}
}

// Range serving and the ancestry shortcut must agree with the header walks on
// random forked chains, after pruning deletes headers (canonical ones included),
// after finalization raises the index floor, after the cap evicts, and after a
// restart empties memory.
func TestBlockIndexMatchesHeaderWalks(t *testing.T) {
	for seed := int64(1); seed <= 25; seed++ {
		rng := rand.New(rand.NewSource(seed))
		backend := storage.NewInMemoryBackend()
		s := NewConsensusStore(backend)
		blocks := randomIndexChain(t, rng, s, 400)
		assertIndexMatchesHeaders(t, rng, s, blocks, "fresh")

		// Delete a few headers, possibly canonical ones.
		var doomed [][32]byte
		for range 5 {
			doomed = append(doomed, blocks[1+rng.Intn(len(blocks)-1)].root)
		}
		pruneBlocksByRoots(s, doomed)
		assertIndexMatchesHeaders(t, rng, s, blocks, "after header prune")
		s.SetHead(blocks[len(blocks)-1].root)
		assertIndexMatchesHeaders(t, rng, s, blocks, "after head moved past prune")

		// Raise the floor as finalization would, then evict as the cap would.
		s.pruneBlockIndexBelow(blocks[len(blocks)/3].slot)
		assertIndexMatchesHeaders(t, rng, s, blocks, "after floor raised")
		s.blocks.mu.Lock()
		s.trimBlockIndexLocked(blocks[len(blocks)/2].slot)
		s.blocks.mu.Unlock()
		assertIndexMatchesHeaders(t, rng, s, blocks, "after eviction")

		restarted := NewConsensusStore(backend)
		assertIndexMatchesHeaders(t, rng, restarted, blocks, "restarted before head set")
		restarted.SetHead(blocks[len(blocks)-2].root)
		assertIndexMatchesHeaders(t, rng, restarted, blocks, "restarted")
	}
}

// After warming up, extending the head must not read the header table: the
// canonical map moves by one slot and the parent is already in memory.
func TestBlockIndexHeadExtensionReadsNothing(t *testing.T) {
	backend := &indexCountingBackend{Backend: storage.NewInMemoryBackend()}
	s := NewConsensusStore(backend)
	parent := [32]byte{0x01}
	storeIndexTestBlock(t, s, parent, types.ZeroRoot, 0)
	s.SetLatestFinalized(&types.Checkpoint{Root: parent})
	s.SetHead(parent)
	for slot := uint64(1); slot <= 3000; slot++ {
		root := [32]byte{0x03, byte(slot), byte(slot >> 8)}
		storeIndexTestBlock(t, s, root, parent, slot)
		before := backend.headerReads
		s.SetHead(root)
		if read := backend.headerReads - before; read != 0 {
			t.Fatalf("slot %d: head extension read %d headers", slot, read)
		}
		parent = root
	}
	// The finalized-to-head ancestry check, run against every vote, is one step.
	before := backend.headerReads
	if !indexedClimb(s, [32]byte{0x01}, 0, parent) {
		t.Fatal("genesis is not an ancestor of the head")
	}
	if read := backend.headerReads - before; read != 0 {
		t.Fatalf("finalized-to-head check read %d headers", read)
	}
}

type indexCountingBackend struct {
	storage.Backend
	headerReads int
}

func (b *indexCountingBackend) BeginRead() (storage.ReadView, error) {
	rv, err := b.Backend.BeginRead()
	if err != nil {
		return nil, err
	}
	return &indexCountingView{ReadView: rv, owner: b}, nil
}

type indexCountingView struct {
	storage.ReadView
	owner *indexCountingBackend
}

func (v *indexCountingView) Get(table storage.Table, key []byte) ([]byte, error) {
	if table == storage.TableBlockHeaders {
		v.owner.headerReads++
	}
	return v.ReadView.Get(table, key)
}

// Attestation validation and range serving read the index from their own
// goroutines while the dispatch loop moves the head and prunes. Run under -race.
func TestBlockIndexConcurrentReadersAndWriter(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	s := NewConsensusStore(storage.NewInMemoryBackend())
	blocks := randomIndexChain(t, rng, s, 200)

	done := make(chan struct{})
	var readers sync.WaitGroup
	for r := 0; r < 4; r++ {
		readers.Add(1)
		go func(seed int64) {
			defer readers.Done()
			rr := rand.New(rand.NewSource(seed))
			for {
				select {
				case <-done:
					return
				default:
				}
				a, d := blocks[rr.Intn(len(blocks))], blocks[rr.Intn(len(blocks))]
				indexedClimb(s, a.root, a.slot, d.root)
				s.GetCanonicalBlocksInRange(uint64(rr.Intn(300)), 20)
			}
		}(int64(r))
	}

	for i := 0; i < 300; i++ {
		s.SetHead(blocks[rng.Intn(len(blocks))].root)
		if i%50 == 0 {
			pruneBlocksByRoots(s, [][32]byte{blocks[1+rng.Intn(len(blocks)-1)].root})
			s.pruneBlockIndexBelow(uint64(i / 10))
		}
	}
	close(done)
	readers.Wait()
}

// Unverified pending headers can carry any slot. Filling the index with them,
// far-future ones included, must keep it within the cap and must not cost the
// canonical chain or any answer.
func TestBlockIndexEvictionResistsUnverifiedHeaders(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	s := NewConsensusStore(storage.NewInMemoryBackend())
	blocks := randomIndexChain(t, rng, s, 300)
	s.SetHead(blocks[len(blocks)-1].root)
	head := blocks[len(blocks)-1]

	for i := 0; i < blockIndexCap+blockIndexCap/2; i++ {
		slot := uint64(rng.Intn(1000))
		if i%1000 == 0 {
			slot = ^uint64(0) - uint64(i)
		}
		s.NoteStoredHeader([32]byte{0x7f, byte(i), byte(i >> 8), byte(i >> 16)}, &types.BlockHeader{Slot: slot})
	}

	s.blocks.mu.RLock()
	size, valid := len(s.blocks.meta), s.blocks.canonicalValid
	s.blocks.mu.RUnlock()
	if size > blockIndexCap {
		t.Fatalf("index holds %d entries, cap %d", size, blockIndexCap)
	}
	if !valid {
		t.Fatal("unverified headers invalidated the canonical chain")
	}
	if _, ok := s.CanonicalAncestorAt(head.root, head.slot, blocks[1].slot); !ok {
		t.Fatal("canonical chain no longer answers for the head")
	}
	assertIndexMatchesHeaders(t, rng, s, blocks, "after unverified flood")
}

// Headers whose parent links do not descend in slot, which the transition never
// admits, must not hang the head update that walks them under the index lock.
func TestBlockIndexHeadMoveTerminatesOnNonDescendingParents(t *testing.T) {
	s := NewConsensusStore(storage.NewInMemoryBackend())
	a, b := [32]byte{0x0a}, [32]byte{0x0b}
	s.InsertBlockHeader(a, &types.BlockHeader{Slot: 5, ParentRoot: b})
	s.InsertBlockHeader(b, &types.BlockHeader{Slot: 5, ParentRoot: a})

	done := make(chan struct{})
	go func() {
		s.SetHead(a)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("head update did not terminate on a parent cycle")
	}
}

// A cache miss during a long head move can push the index over its cap. The
// eviction that follows must not trim canonical slots the walk has already
// written, or range serving reads the holes as empty slots and drops blocks.
func TestBlockIndexEvictionDuringHeadMoveKeepsRangesWhole(t *testing.T) {
	s := NewConsensusStore(storage.NewInMemoryBackend())
	genesis := [32]byte{0x01}
	storeIndexTestBlock(t, s, genesis, types.ZeroRoot, 0)
	s.SetLatestFinalized(&types.Checkpoint{Root: genesis})
	a1 := [32]byte{0xa1}
	storeIndexTestBlock(t, s, a1, genesis, 70000)
	s.SetHead(a1)

	b1, b2, b3 := [32]byte{0xb1}, [32]byte{0xb2}, [32]byte{0xb3}
	storeIndexTestBlock(t, s, b1, genesis, 5)
	storeIndexTestBlock(t, s, b2, b1, 100)
	storeIndexTestBlock(t, s, b3, b2, 70001)
	// Fill the cache to the cap, then drop b1 so the walk misses on it.
	for i := 0; len(s.blocks.meta) < blockIndexCap; i++ {
		s.NoteStoredHeader([32]byte{0x7e, byte(i), byte(i >> 8), byte(i >> 16)}, &types.BlockHeader{Slot: uint64(i % 1000)})
	}
	s.blocks.mu.Lock()
	delete(s.blocks.meta, b1)
	s.blocks.meta[[32]byte{0x7d}] = blockMeta{slot: 1}
	full, valid := len(s.blocks.meta) == blockIndexCap, s.blocks.canonicalValid
	s.blocks.mu.Unlock()
	if !full || !valid {
		t.Fatalf("setup: cache full=%t canonical valid=%t, want both", full, valid)
	}

	s.SetHead(b3)

	got, complete := s.GetCanonicalBlocksInRange(50, 100)
	want, wantComplete := headerWalkRange(s, 50, 100)
	if complete != wantComplete || len(got) != len(want) {
		t.Fatalf("range [50,150) = %d blocks complete=%t, walk gives %d complete=%t", len(got), complete, len(want), wantComplete)
	}
}
