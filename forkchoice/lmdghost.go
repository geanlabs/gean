// Package forkchoice implements the LMD GHOST fork choice algorithm.
package forkchoice

import "github.com/devylongs/gean/types"

func GetHead(blocks map[types.Root]*types.Block, root types.Root, latestVotes []types.Checkpoint, minScore int) types.Root {
	if root.IsZero() {
		var minSlot types.Slot = ^types.Slot(0)
		for hash, block := range blocks {
			if block.Slot < minSlot {
				minSlot = block.Slot
				root = hash
			}
		}
	}

	voteWeights := make(map[types.Root]int)
	rootSlot := blocks[root].Slot

	for _, vote := range latestVotes {
		if vote.Root.IsZero() {
			continue
		}
		if _, exists := blocks[vote.Root]; !exists {
			continue
		}
		blockHash := vote.Root
		for blocks[blockHash].Slot > rootSlot {
			voteWeights[blockHash]++
			blockHash = blocks[blockHash].ParentRoot
		}
	}

	// Build children mapping for blocks above min score
	// When minScore is 0, include all children (even those without votes)
	childrenMap := make(map[types.Root][]types.Root)
	for blockHash, block := range blocks {
		if block.ParentRoot.IsZero() {
			continue
		}
		if minScore == 0 || voteWeights[blockHash] >= minScore {
			childrenMap[block.ParentRoot] = append(childrenMap[block.ParentRoot], blockHash)
		}
	}

	// Walk down tree, choosing child with most votes
	current := root
	for {
		children := childrenMap[current]
		if len(children) == 0 {
			return current
		}

		// Choose best child: most votes, then highest slot, then highest hash
		best := children[0]
		bestWeight := voteWeights[best]
		bestSlot := blocks[best].Slot

		for _, child := range children[1:] {
			weight := voteWeights[child]
			childSlot := blocks[child].Slot

			// Tie-break: most votes, then highest slot, then lexicographically highest hash
			if weight > bestWeight ||
				(weight == bestWeight && childSlot > bestSlot) ||
				(weight == bestWeight && childSlot == bestSlot && child.Compare(best) > 0) {
				best = child
				bestWeight = weight
				bestSlot = childSlot
			}
		}

		current = best
	}
}

// GetLatestJustified finds the justified checkpoint with the highest slot.
func GetLatestJustified(states map[types.Root]*types.State) *types.Checkpoint {
	if len(states) == 0 {
		return nil
	}

	var latest *types.Checkpoint
	var latestSlot types.Slot

	for _, state := range states {
		if latest == nil || state.LatestJustified.Slot > latestSlot {
			cp := state.LatestJustified
			latest = &cp
			latestSlot = cp.Slot
		}
	}

	return latest
}

