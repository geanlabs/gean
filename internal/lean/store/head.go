package store

import (
	"github.com/geanlabs/gean/internal/lean/forkchoice"
	"github.com/geanlabs/gean/internal/lean/types"
)

// ComputeHead feeds the latest known votes into fork choice and returns the
// head under the latest justified checkpoint.
func ComputeHead(s *ConsensusStore, fc *forkchoice.ForkChoice) [32]byte {
	for validatorID, data := range s.ExtractLatestKnownAttestations() {
		fc.SetKnownVote(validatorID, data.Head.Root, data.Slot, data)
	}
	return fc.UpdateHead(s.LatestJustified().Root)
}

// UpdateSafeTarget recomputes the safe target from the latest new votes and
// stores it, reporting false when the head's state is unknown. It leaves fork
// choice's weights at the supermajority threshold, so it runs after the head is
// computed for the current votes.
func UpdateSafeTarget(s *ConsensusStore, fc *forkchoice.ForkChoice) ([32]byte, bool) {
	headSummary, ok := s.StateSummary(s.Head())
	if !ok {
		return types.ZeroRoot, false
	}
	safeTarget := fc.UpdateSafeTarget(s.LatestJustified().Root, headSummary.NumValidators, s.ExtractLatestNewAttestations())
	s.SetSafeTarget(safeTarget)
	return safeTarget, true
}
