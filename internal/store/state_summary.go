package store

import "github.com/geanlabs/gean/internal/types"

// StateSummary is the part of a stored state that the per-slot paths read: the
// head update, attestation production, proposer and safe-target checks. They used
// to decode the whole head state for these few values — several megabytes on a
// long-lived chain, several times a slot — and the head update additionally walked
// the chain to finalization on disk. The summary is recorded when the state is
// written, so those paths read memory instead.
//
// It is a value: callers get their own copy and cannot change what is held.
type StateSummary struct {
	Finalized     types.Checkpoint
	Justified     types.Checkpoint
	NumValidators uint64
}

// StateSummary returns the summary of the stored state at root. A miss — a state
// written before a restart — decodes that state once and keeps the summary.
func (s *ConsensusStore) StateSummary(root [32]byte) (StateSummary, bool) {
	if s == nil {
		return StateSummary{}, false
	}
	s.stateSummariesMu.Lock()
	summary, ok := s.stateSummaries[root]
	s.stateSummariesMu.Unlock()
	if ok {
		return summary, true
	}

	state := s.GetState(root)
	if !s.noteStateSummary(root, state) {
		return StateSummary{}, false
	}
	return summarize(state), true
}

// NoteStoredState records the summary of a state written outside PutState, such
// as the block-import batch.
func (s *ConsensusStore) NoteStoredState(root [32]byte, state *types.State) {
	s.noteStateSummary(root, state)
}

// noteStateSummary records the summary of a state stored under root. A state
// missing any part the summary needs is not recorded, so readers fall back to
// the state itself and see the same gap they always did.
func (s *ConsensusStore) noteStateSummary(root [32]byte, state *types.State) bool {
	if s == nil || state == nil || state.LatestBlockHeader == nil ||
		state.LatestJustified == nil || state.LatestFinalized == nil {
		return false
	}
	s.stateSummariesMu.Lock()
	defer s.stateSummariesMu.Unlock()
	if s.stateSummaries == nil {
		s.stateSummaries = make(map[[32]byte]StateSummary)
	}
	s.stateSummaries[root] = summarize(state)
	return true
}

func summarize(state *types.State) StateSummary {
	return StateSummary{
		Finalized:     *state.LatestFinalized,
		Justified:     *state.LatestJustified,
		NumValidators: state.NumValidators(),
	}
}

// forgetStateSummaries drops the summaries of states that have been deleted, so
// the map never holds more than the states table does.
func (s *ConsensusStore) forgetStateSummaries(roots [][32]byte) {
	s.stateSummariesMu.Lock()
	defer s.stateSummariesMu.Unlock()
	for _, root := range roots {
		delete(s.stateSummaries, root)
	}
}
