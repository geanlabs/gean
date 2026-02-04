// Package consensus implements justification tracking helpers for the Lean Ethereum spec.
package consensus

import (
	"sort"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/devylongs/gean/types"
)

// GetJustifications reconstructs the justifications map from the flattened state encoding.
// Returns a map from block root to a slice of validator votes (true = voted).
//
// Per leanSpec chain.md:
// - state.justifications_roots contains block roots under voting consideration
// - state.justifications_validators is a flattened bitlist of validator votes
func GetJustifications(s *types.State) map[types.Root][]bool {
	justifications := make(map[types.Root][]bool)

	if len(s.JustificationRoots) == 0 {
		return justifications
	}

	numValidators := int(s.Config.NumValidators)
	flatVotes := bitfield.Bitlist(s.JustificationValidators)

	for i, root := range s.JustificationRoots {
		// Calculate the starting index for this root's votes
		startIdx := i * numValidators

		// Extract votes for this root
		votes := make([]bool, numValidators)
		for j := 0; j < numValidators; j++ {
			idx := uint64(startIdx + j)
			if idx < flatVotes.Len() {
				votes[j] = flatVotes.BitAt(idx)
			}
		}

		justifications[root] = votes
	}

	return justifications
}

// SetJustifications flattens the justifications map back into the state's SSZ-compatible format.
//
// Per leanSpec chain.md:
// - Roots are stored in sorted order for deterministic encoding
// - Validator votes are flattened into a single bitlist
func SetJustifications(s *types.State, justifications map[types.Root][]bool) *types.State {
	newState := Copy(s)

	if len(justifications) == 0 {
		newState.JustificationRoots = []types.Root{}
		newState.JustificationValidators = bitfield.NewBitlist(1) // Empty bitlist with delimiter
		return newState
	}

	// Collect and sort roots for deterministic order
	roots := make([]types.Root, 0, len(justifications))
	for root := range justifications {
		roots = append(roots, root)
	}
	sortRoots(roots)

	// Calculate total bits needed
	numValidators := int(s.Config.NumValidators)
	totalBits := len(roots) * numValidators

	// Create the flattened bitlist
	flatVotes := bitfield.NewBitlist(uint64(totalBits))

	// Flatten all votes
	for i, root := range roots {
		votes := justifications[root]
		for j, voted := range votes {
			if voted {
				flatVotes.SetBitAt(uint64(i*numValidators+j), true)
			}
		}
	}

	newState.JustificationRoots = roots
	newState.JustificationValidators = flatVotes

	return newState
}

// sortRoots sorts roots lexicographically for deterministic ordering.
func sortRoots(roots []types.Root) {
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].Compare(roots[j]) < 0
	})
}

// CountVotes counts the number of true votes in a vote slice.
func CountVotes(votes []bool) int {
	count := 0
	for _, v := range votes {
		if v {
			count++
		}
	}
	return count
}
