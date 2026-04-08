package forkchoice

import "github.com/geanlabs/gean/types"

// Spec-compliant LMD GHOST implementation for testing.
// Used as debug oracle to validate proto-array produces identical results.

// SpecComputeBlockWeights computes per-block attestation weights.
// For each attestation, walks backward from head through parent chain,
// incrementing weight for each block above startSlot.
func SpecComputeBlockWeights(
	startSlot uint64,
	blocks map[[32]byte]BlockInfo,
	attestations map[uint64]*types.AttestationData,
) map[[32]byte]uint64 {
	weights := make(map[[32]byte]uint64)

	for _, data := range attestations {
		current := data.Head.Root
		for {
			info, ok := blocks[current]
			if !ok || info.Slot <= startSlot {
				break
			}
			weights[current]++
			current = info.ParentRoot
		}
	}

	return weights
}

// SpecComputeLMDGhostHead computes the LMD GHOST head.
func SpecComputeLMDGhostHead(
	startRoot [32]byte,
	blocks map[[32]byte]BlockInfo,
	attestations map[uint64]*types.AttestationData,
	minScore uint64,
) ([32]byte, map[[32]byte]uint64) {
	if len(blocks) == 0 {
		return startRoot, nil
	}

	// If start root is zero, use the block with the lowest slot.
	if startRoot == [32]byte{} {
		var minSlot uint64 = ^uint64(0)
		for root, info := range blocks {
			if info.Slot < minSlot {
				minSlot = info.Slot
				startRoot = root
			}
		}
	}

	startInfo, ok := blocks[startRoot]
	if !ok {
		return startRoot, nil
	}
	startSlot := startInfo.Slot

	weights := SpecComputeBlockWeights(startSlot, blocks, attestations)

	// Build children map, filtering by min_score.
	children := make(map[[32]byte][][32]byte)
	for root, info := range blocks {
		if info.ParentRoot == [32]byte{} {
			continue
		}
		if minScore > 0 {
			w := weights[root]
			if w < minScore {
				continue
			}
		}
		children[info.ParentRoot] = append(children[info.ParentRoot], root)
	}

	// Greedy descent: pick best child (most weight, then lexicographic).
	head := startRoot
	for {
		kids, ok := children[head]
		if !ok || len(kids) == 0 {
			break
		}
		best := kids[0]
		bestWeight := weights[best]
		for _, kid := range kids[1:] {
			w := weights[kid]
			if w > bestWeight {
				best = kid
				bestWeight = w
			} else if w == bestWeight {
				// Lexicographic tiebreak: larger root wins.
				if rootGreaterThan(kid, best) {
					best = kid
					bestWeight = w
				}
			}
		}
		head = best
	}

	return head, weights
}

// BlockInfo is the minimal block data for spec fork choice.
type BlockInfo struct {
	Slot       uint64
	ParentRoot [32]byte
}

func rootGreaterThan(a, b [32]byte) bool {
	for i := 0; i < 32; i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
