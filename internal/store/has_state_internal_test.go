package store

import (
	"errors"
	"testing"

	"github.com/geanlabs/gean/internal/storage"
)

// HasState answers from the summary or by key, never by reading the state, and
// its answer always matches what the states table holds: after a write, after
// a restart has emptied memory, and after a prune.
func TestHasStateNeverReadsStateValues(t *testing.T) {
	backend := &tableReadCounter{Backend: storage.NewInMemoryBackend(), table: storage.TableStates}
	s := NewConsensusStore(backend)
	stored, other := [32]byte{0x01}, [32]byte{0x02}
	s.InsertState(stored, summaryTestState(3, 4))
	s.InsertState(other, summaryTestState(3, 4))

	restarted := NewConsensusStore(backend)
	for name, st := range map[string]*ConsensusStore{"live": s, "restarted": restarted} {
		if !st.HasState(stored) {
			t.Fatalf("%s: stored state reported missing", name)
		}
		if st.HasState([32]byte{0xee}) {
			t.Fatalf("%s: missing state reported stored", name)
		}
	}

	pruneStatesByRoots(s, [][32]byte{stored})
	for name, st := range map[string]*ConsensusStore{"live": s, "restarted": restarted} {
		if st.HasState(stored) {
			t.Fatalf("%s: pruned state reported stored", name)
		}
		if !st.HasState(other) {
			t.Fatalf("%s: surviving state reported missing", name)
		}
	}
	if backend.reads != 0 {
		t.Fatalf("existence checks read %d state values, want 0", backend.reads)
	}
}

type failingDeleteBackend struct {
	storage.Backend
}

func (b failingDeleteBackend) BeginWrite() (storage.WriteBatch, error) {
	return nil, errors.New("begin write failed")
}

// A summary is trusted as proof the state is stored, so a prune drops it before
// deleting. If the delete then fails, the state is still there and HasState must
// still find it.
func TestHasStateAfterFailedPruneStillFindsState(t *testing.T) {
	mem := storage.NewInMemoryBackend()
	s := NewConsensusStore(mem)
	root := [32]byte{0x01}
	s.InsertState(root, summaryTestState(3, 4))

	s.Backend = failingDeleteBackend{Backend: mem}
	if n := pruneStatesByRoots(s, [][32]byte{root}); n != 0 {
		t.Fatalf("prune reported %d deletions with a failing backend", n)
	}
	if !s.HasState(root) {
		t.Fatal("state survived a failed prune but HasState reports it missing")
	}
	if _, ok := s.StateSummary(root); !ok {
		t.Fatal("summary of a surviving state is not recomputed")
	}
}
