package store

import (
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/types"
)

func summaryTestState(finalizedSlot uint64, validators int) *types.State {
	st := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		LatestBlockHeader:        &types.BlockHeader{},
		LatestJustified:          &types.Checkpoint{Root: [32]byte{0x08}, Slot: finalizedSlot + 1},
		LatestFinalized:          &types.Checkpoint{Root: [32]byte{0x09}, Slot: finalizedSlot},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
	for i := 0; i < validators; i++ {
		st.Validators = append(st.Validators, &types.Validator{Index: uint64(i)})
	}
	return st
}

// The summaries must track the states table: one is added when a state is
// written and dropped when the state is pruned, so the map can never outgrow
// the states it describes.
func TestStateSummariesFollowStatesTable(t *testing.T) {
	s := NewConsensusStore(storage.NewInMemoryBackend())
	kept, pruned := [32]byte{0x01}, [32]byte{0x02}
	s.InsertState(kept, summaryTestState(3, 4))
	s.InsertState(pruned, summaryTestState(3, 4))
	if len(s.stateSummaries) != 2 {
		t.Fatalf("summaries after two writes = %d, want 2", len(s.stateSummaries))
	}

	if n := pruneStatesByRoots(s, [][32]byte{pruned}); n != 1 {
		t.Fatalf("pruned %d states, want 1", n)
	}
	if _, ok := s.stateSummaries[pruned]; ok {
		t.Fatal("pruned state's summary is still held")
	}
	if _, ok := s.stateSummaries[kept]; !ok {
		t.Fatal("kept state's summary was dropped")
	}
}

// A summary must report exactly what the stored state holds, whether it comes
// from memory or, after a restart, from decoding the state.
func TestStateSummaryMatchesStoredState(t *testing.T) {
	backend := storage.NewInMemoryBackend()
	live := NewConsensusStore(backend)
	root := [32]byte{0x01}
	st := summaryTestState(7, 5)
	live.InsertState(root, st)
	want := StateSummary{Finalized: *st.LatestFinalized, Justified: *st.LatestJustified, NumValidators: 5}

	restarted := NewConsensusStore(backend)
	if len(restarted.stateSummaries) != 0 {
		t.Fatal("a fresh store must start with no summaries")
	}
	for name, s := range map[string]*ConsensusStore{"live": live, "restarted": restarted} {
		got, ok := s.StateSummary(root)
		if !ok || got != want {
			t.Fatalf("%s: summary = %+v (ok=%t), want %+v", name, got, ok, want)
		}
	}
	if _, ok := restarted.stateSummaries[root]; ok {
		t.Fatal("a summary decoded on a miss was cached; only writes may create one")
	}
	if _, ok := live.StateSummary([32]byte{0xee}); ok {
		t.Fatal("summary reported for a root with no stored state")
	}
}
