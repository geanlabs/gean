package transition

import (
	"sort"

	"github.com/geanlabs/gean/internal/lean/types"
)

func reconstructJustifications(state *types.State, validatorCount uint64) map[[32]byte][]bool {
	justifications := make(map[[32]byte][]bool)
	if state == nil || validatorCount == 0 {
		return justifications
	}
	for row := range uint64(len(state.JustificationsRoots)) {
		var root [32]byte
		copy(root[:], state.JustificationsRoots[row])
		votes := make([]bool, validatorCount)
		for v := range validatorCount {
			if types.BitlistGet(state.JustificationsValidators, row*validatorCount+v) {
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
	validatorCount := uint64(len(state.Validators))
	if validatorCount == 0 || types.BitlistLen(state.JustificationsValidators) != uint64(len(state.JustificationsRoots))*validatorCount {
		return nil
	}
	for row := range uint64(len(state.JustificationsRoots)) {
		if [32]byte(state.JustificationsRoots[row]) != root {
			continue
		}
		votes := make([]bool, validatorCount)
		for v := range validatorCount {
			votes[v] = types.BitlistGet(state.JustificationsValidators, row*validatorCount+v)
		}
		return votes
	}
	return nil
}

func serializeJustifications(state *types.State, justifications map[[32]byte][]bool, validatorCount uint64) {
	roots := sortedJustificationRoots(justifications)
	sszRoots := make([][]byte, len(roots))
	for i, root := range roots {
		r := make([]byte, types.RootSize)
		copy(r, root[:])
		sszRoots[i] = r
	}
	state.JustificationsRoots = sszRoots

	totalBits := uint64(len(roots)) * validatorCount
	if totalBits == 0 {
		state.JustificationsValidators = types.NewBitlistSSZ(0)
		return
	}

	bits := types.NewBitlistSSZ(totalBits)
	for row := range uint64(len(roots)) {
		votes := justifications[roots[row]]
		for v := range validatorCount {
			if v < uint64(len(votes)) && votes[v] {
				types.BitlistSet(bits, row*validatorCount+v)
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
