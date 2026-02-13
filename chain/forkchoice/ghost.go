package forkchoice

import (
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

// GetForkChoiceHead uses LMD GHOST to find the head block from a given root.
func GetForkChoiceHead(
	store storage.Store,
	root [32]byte,
	latestAttestations map[uint64]*types.Attestation,
	minScore int,
) [32]byte {
	blocks := store.GetAllBlocks()

	// Start at earliest block if root is zero hash.
	if root == types.ZeroHash {
		var earliest [32]byte
		minSlot := uint64(^uint64(0))
		for h, b := range blocks {
			if b.Slot < minSlot {
				minSlot = b.Slot
				earliest = h
			}
		}
		root = earliest
	}

	if len(latestAttestations) == 0 {
		return root
	}

	rootBlock, ok := blocks[root]
	if !ok {
		return root
	}
	rootSlot := rootBlock.Slot

	// Count votes for each block. Votes for descendants count toward ancestors.
	voteWeights := make(map[[32]byte]int)
	for _, att := range latestAttestations {
		headRoot := att.Data.Head.Root
		if _, ok := blocks[headRoot]; !ok {
			continue
		}
		blockHash := headRoot
		for {
			b, exists := blocks[blockHash]
			if !exists || b.Slot <= rootSlot {
				break
			}
			voteWeights[blockHash]++
			blockHash = b.ParentRoot
		}
	}

	// Build children mapping for blocks above min score.
	childrenMap := make(map[[32]byte][][32]byte)
	for blockHash := range blocks {
		block := blocks[blockHash]
		if voteWeights[blockHash] >= minScore {
			childrenMap[block.ParentRoot] = append(childrenMap[block.ParentRoot], blockHash)
		}
	}

	// Walk down tree, choosing child with most votes.
	// Tiebreak: highest slot, then largest hash.
	current := root
	for {
		children := childrenMap[current]
		if len(children) == 0 {
			return current
		}

		best := children[0]
		bestWeight := voteWeights[best]
		bestSlot := blocks[best].Slot
		for _, c := range children[1:] {
			w := voteWeights[c]
			s := blocks[c].Slot
			if w > bestWeight || (w == bestWeight && s > bestSlot) || (w == bestWeight && s == bestSlot && hashGreater(c, best)) {
				best = c
				bestWeight = w
				bestSlot = s
			}
		}
		current = best
	}
}

// GetLatestJustified finds the justified checkpoint with the highest slot.
func GetLatestJustified(store storage.Store) *types.Checkpoint {
	states := store.GetAllStates()
	if len(states) == 0 {
		return nil
	}

	var latest *types.Checkpoint
	for _, s := range states {
		if latest == nil || s.LatestJustified.Slot > latest.Slot {
			latest = s.LatestJustified
		}
	}
	return latest
}

func hashGreater(a, b [32]byte) bool {
	for i := 0; i < 32; i++ {
		if a[i] > b[i] {
			return true
		}
		if a[i] < b[i] {
			return false
		}
	}
	return false
}
