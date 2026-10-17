package statetransition

import (
	"sort"

	"github.com/geanlabs/gean/internal/types"
)

func reconstructJustifications(state *types.State, validatorCount int) map[[32]byte][]bool {
	justifications := make(map[[32]byte][]bool)
	if state == nil || validatorCount <= 0 {
		return justifications
	}
	for i, rootBytes := range state.JustificationsRoots {
		var root [32]byte
		copy(root[:], rootBytes)
		votes := make([]bool, validatorCount)
		for v := range validatorCount {
			if types.BitlistGet(state.JustificationsValidators, uint64(i*validatorCount+v)) {
				votes[v] = true
			}
		}
		justifications[root] = votes
	}
	return justifications
}

// JustificationVotes returns one tracked target's tally: which validators have a
// counted vote for root. Nil when the root is not tracked or the state is
// malformed. Reads a single row, where reconstructJustifications decodes every
// tracked root.
func JustificationVotes(state *types.State, root [32]byte) []bool {
	if state == nil {
		return nil
	}
	validatorCount := len(state.Validators)
	if validatorCount == 0 || int(types.BitlistLen(state.JustificationsValidators)) != len(state.JustificationsRoots)*validatorCount {
		return nil
	}
	for i, rootBytes := range state.JustificationsRoots {
		if [32]byte(rootBytes) != root {
			continue
		}
		votes := make([]bool, validatorCount)
		for v := range validatorCount {
			votes[v] = types.BitlistGet(state.JustificationsValidators, uint64(i*validatorCount+v))
		}
		return votes
	}
	return nil
}

func serializeJustifications(state *types.State, justifications map[[32]byte][]bool, validatorCount int) {
	roots := sortedJustificationRoots(justifications)
	sszRoots := make([][]byte, len(roots))
	for i, root := range roots {
		r := make([]byte, types.RootSize)
		copy(r, root[:])
		sszRoots[i] = r
	}
	state.JustificationsRoots = sszRoots

	totalBits := uint64(len(roots)) * uint64(validatorCount)
	if totalBits == 0 {
		state.JustificationsValidators = types.NewBitlistSSZ(0)
		return
	}

	bits := types.NewBitlistSSZ(totalBits)
	for i, root := range roots {
		votes := justifications[root]
		for v := range validatorCount {
			if v < len(votes) && votes[v] {
				types.BitlistSet(bits, uint64(i*validatorCount+v))
			}
		}
	}
	state.JustificationsValidators = bits
}

func sortedJustificationRoots(justifications map[[32]byte][]bool) [][32]byte {
	roots := make([][32]byte, 0, len(justifications))
	for root := range justifications {
		roots = append(roots, root)
	}
	sort.Slice(roots, func(i, j int) bool {
		for k := range 32 {
			if roots[i][k] != roots[j][k] {
				return roots[i][k] < roots[j][k]
			}
		}
		return false
	})
	return roots
}
