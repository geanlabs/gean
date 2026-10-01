package attestation

import (
	"math/rand"
	"testing"

	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// headerClimbIsAncestor is checkpointIsAncestor as it was before the block index:
// the spec's climb, one header read per step.
func headerClimbIsAncestor(s *store.ConsensusStore, ancestor, descendant *types.Checkpoint) bool {
	if ancestor.Slot > descendant.Slot {
		return false
	}
	current := descendant.Root
	for {
		header := s.GetBlockHeader(current)
		if header == nil {
			return false
		}
		if header.Slot == ancestor.Slot {
			return current == ancestor.Root
		}
		if header.Slot < ancestor.Slot {
			return false
		}
		current = header.ParentRoot
	}
}

// headerWalkTarget is GetAttestationTarget as it was before the block index.
func headerWalkTarget(s *store.ConsensusStore) *types.Checkpoint {
	targetRoot := s.Head()
	targetHeader := s.GetBlockHeader(targetRoot)
	if targetHeader == nil {
		return &types.Checkpoint{}
	}
	safeTargetSlot := uint64(0)
	if safeTargetHeader := s.GetBlockHeader(s.SafeTarget()); safeTargetHeader != nil {
		safeTargetSlot = safeTargetHeader.Slot
	}
	finalizedSlot := s.LatestFinalized().Slot
	lowerBoundSlot := max(safeTargetSlot, finalizedSlot)
	for range uint64(types.JustificationLookbackSlots) {
		if targetHeader.Slot <= lowerBoundSlot {
			break
		}
		targetRoot = targetHeader.ParentRoot
		parent := s.GetBlockHeader(targetRoot)
		if parent == nil {
			break
		}
		targetHeader = parent
	}
	for targetHeader.Slot > finalizedSlot &&
		!statetransition.SlotIsJustifiableAfter(targetHeader.Slot, finalizedSlot) {
		targetRoot = targetHeader.ParentRoot
		parent := s.GetBlockHeader(targetRoot)
		if parent == nil {
			break
		}
		targetHeader = parent
	}
	return &types.Checkpoint{Root: targetRoot, Slot: targetHeader.Slot}
}

type walkBlock struct {
	root [32]byte
	slot uint64
}

// Random block trees, with forks, skipped slots, reorgs, a head moving down and
// back, a checkpoint-sync-style anchor whose parent is never stored, and
// restarts that empty the in-memory index: at every step the index-backed
// answers must equal the header climbs they replace.
func TestIndexedWalksMatchHeaderClimbs(t *testing.T) {
	for seed := int64(1); seed <= 40; seed++ {
		rng := rand.New(rand.NewSource(seed))
		backend := storage.NewInMemoryBackend()
		s := store.NewConsensusStore(backend)
		s.SetConfig(&types.ChainConfig{GenesisTime: 1000})

		anchorSlot := uint64(0)
		var anchorParent [32]byte
		if seed%3 == 0 {
			// Anchor above genesis with a parent that is never stored.
			anchorSlot = uint64(5 + rng.Intn(20))
			anchorParent = [32]byte{0xee, byte(seed)}
		}
		anchor := [32]byte{0xaa, byte(seed)}
		s.InsertBlockHeader(anchor, &types.BlockHeader{Slot: anchorSlot, ParentRoot: anchorParent})
		s.SetLatestFinalized(&types.Checkpoint{Root: anchor, Slot: anchorSlot})
		s.SetSafeTarget(anchor)
		s.SetHead(anchor)
		blocks := []walkBlock{{root: anchor, slot: anchorSlot}}

		for step := 0; step < 300; step++ {
			// Extend a recent block, skipping up to two slots.
			parent := blocks[len(blocks)-1-rng.Intn(min(len(blocks), 6))]
			child := walkBlock{root: [32]byte{0xbb, byte(seed), byte(step), byte(step >> 8)}, slot: parent.slot + 1 + uint64(rng.Intn(3))}
			s.InsertBlockHeader(child.root, &types.BlockHeader{Slot: child.slot, ParentRoot: parent.root})
			blocks = append(blocks, child)

			switch r := rng.Intn(10); {
			case r < 6:
				s.SetHead(child.root)
			case r < 8:
				s.SetHead(blocks[rng.Intn(len(blocks))].root)
			case r == 8:
				s.SetSafeTarget(blocks[rng.Intn(len(blocks))].root)
			default:
				// Move finalization to an ancestor of the head.
				head := s.Head()
				for _, b := range blocks {
					cp := &types.Checkpoint{Root: b.root, Slot: b.slot}
					if b.slot > s.LatestFinalized().Slot && headerClimbIsAncestor(s, cp, &types.Checkpoint{Root: head, Slot: blockSlot(blocks, head)}) && rng.Intn(4) == 0 {
						s.SetLatestFinalized(cp)
						break
					}
				}
			}
			if rng.Intn(50) == 0 {
				head := s.Head()
				s = store.NewConsensusStore(backend)
				if rng.Intn(2) == 0 {
					s.SetHead(head)
				}
			}

			for range 20 {
				a, d := blocks[rng.Intn(len(blocks))], blocks[rng.Intn(len(blocks))]
				anc := &types.Checkpoint{Root: a.root, Slot: a.slot}
				if rng.Intn(5) == 0 {
					// A checkpoint whose slot does not match its block.
					anc.Slot += uint64(rng.Intn(3))
				}
				desc := &types.Checkpoint{Root: d.root, Slot: d.slot}
				if got, want := checkpointIsAncestor(s, anc, desc), headerClimbIsAncestor(s, anc, desc); got != want {
					t.Fatalf("seed %d step %d: ancestor(%d->%d) = %t, climb gives %t", seed, step, a.slot, d.slot, got, want)
				}
			}
			if got, want := GetAttestationTarget(s), headerWalkTarget(s); *got != *want {
				t.Fatalf("seed %d step %d: target = %+v, walk gives %+v", seed, step, got, want)
			}
		}
	}
}

func blockSlot(blocks []walkBlock, root [32]byte) uint64 {
	for _, b := range blocks {
		if b.root == root {
			return b.slot
		}
	}
	return 0
}
