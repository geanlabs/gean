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
// written before a restart — decodes the state and is not cached: summaries are
// created only when a state is written, so HasState can take one as proof the
// state is stored. A reader caching on a miss could race a prune and leave a
// summary behind for a deleted state. Once the head moves to a block imported
// after the restart, every state the per-slot paths ask about has a summary;
// until then they decode the head state as they did before summaries existed.
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

	return summaryOf(s.GetState(root))
}

// NoteStoredState records the summary of a state just stored under root. PutState
// calls it; a writer that bypasses PutState, such as the block-import batch, must
// call it after its commit.
func (s *ConsensusStore) NoteStoredState(root [32]byte, state *types.State) {
	summary, ok := summaryOf(state)
	if s == nil || !ok {
		return
	}
	s.stateSummariesMu.Lock()
	defer s.stateSummariesMu.Unlock()
	if s.stateSummaries == nil {
		s.stateSummaries = make(map[[32]byte]StateSummary)
	}
	s.stateSummaries[root] = summary
}

// summaryOf extracts a state's summary. A state missing any part the summary
// needs has none, so readers fall back to the state itself and see the same gap
// they always did.
func summaryOf(state *types.State) (StateSummary, bool) {
	if state == nil || state.LatestBlockHeader == nil ||
		state.LatestJustified == nil || state.LatestFinalized == nil {
		return StateSummary{}, false
	}
	return StateSummary{
		Finalized:     *state.LatestFinalized,
		Justified:     *state.LatestJustified,
		NumValidators: state.NumValidators(),
	}, true
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
