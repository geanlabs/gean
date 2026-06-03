package forkchoice

import (
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

func SpecComputeBlockWeights(
	startSlot uint64,
	blocks map[[32]byte]BlockInfo,
	attestations map[uint64]*types.AttestationData,
) map[[32]byte]uint64 {
	weights := make(map[[32]byte]uint64)

	for _, data := range attestations {
		if data == nil || data.Head == nil {
			continue
		}
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

func SpecComputeLMDGhostHead(
	startRoot [32]byte,
	blocks map[[32]byte]BlockInfo,
	attestations map[uint64]*types.AttestationData,
	minScore uint64,
) ([32]byte, map[[32]byte]uint64, error) {
	if len(blocks) == 0 {
		return startRoot, nil, nil
	}

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
		return [32]byte{}, nil, fmt.Errorf("start root %x not in blocks", startRoot)
	}
	weights := SpecComputeBlockWeights(startInfo.Slot, blocks, attestations)

	children := make(map[[32]byte][][32]byte)
	for root, info := range blocks {
		if info.ParentRoot == [32]byte{} {
			continue
		}
		if minScore > 0 && weights[root] < minScore {
			continue
		}
		children[info.ParentRoot] = append(children[info.ParentRoot], root)
	}

	head := startRoot
	for {
		kids := children[head]
		if len(kids) == 0 {
			break
		}
		best := kids[0]
		bestWeight := weights[best]
		for _, kid := range kids[1:] {
			weight := weights[kid]
			if weight > bestWeight || (weight == bestWeight && rootGreaterThan(kid, best)) {
				best = kid
				bestWeight = weight
			}
		}
		head = best
	}

	return head, weights, nil
}

type BlockInfo struct {
	Slot       uint64
	ParentRoot [32]byte
}

func rootGreaterThan(a, b [32]byte) bool {
	for i := range 32 {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
