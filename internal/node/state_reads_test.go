package node

import (
	"sync/atomic"
	"testing"

	"github.com/geanlabs/gean/internal/attestation"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/types"
)

// stateReadCounter counts reads of the states table, the multi-megabyte values
// the per-slot paths must not decode.
type stateReadCounter struct {
	storage.Backend
	reads atomic.Int64
}

func (b *stateReadCounter) BeginRead() (storage.ReadView, error) {
	rv, err := b.Backend.BeginRead()
	if err != nil {
		return nil, err
	}
	return &stateReadCounterView{ReadView: rv, owner: b}, nil
}

type stateReadCounterView struct {
	storage.ReadView
	owner *stateReadCounter
}

func (v *stateReadCounterView) Get(table storage.Table, key []byte) ([]byte, error) {
	if table == storage.TableStates {
		v.owner.reads.Add(1)
	}
	return v.ReadView.Get(table, key)
}

// The paths that run every slot read the head state's checkpoints and
// validator count from memory. Decoding the head state for them cost several
// milliseconds each on a long-lived chain, several times a slot, all on the
// dispatch loop.
func TestPerSlotPathsDoNotDecodeHeadState(t *testing.T) {
	e := makeTestEngine()
	head := e.Store.Head()
	headState := e.Store.GetState(head)
	headState.Validators = make([]*types.Validator, 7)
	for i := range headState.Validators {
		headState.Validators[i] = &types.Validator{Index: uint64(i)}
	}
	justified := types.Checkpoint{Root: head, Slot: 0}
	headState.LatestJustified = &justified
	e.Store.InsertState(head, headState)

	counter := &stateReadCounter{Backend: e.Store.Backend}
	e.Store.Backend = counter

	e.updateHead()
	e.updateSafeTarget()
	if n := e.coverageValidatorCount(); n != 7 {
		t.Fatalf("validator count = %d, want the head state's 7", n)
	}
	data := attestation.ProduceAttestationData(e.Store, 1)
	if data == nil {
		t.Fatal("attestation data not produced")
	}
	if *data.Source != justified {
		t.Fatalf("source = %+v, want the head state's justified %+v", *data.Source, justified)
	}

	if got := counter.reads.Load(); got != 0 {
		t.Fatalf("per-slot paths read %d states from storage, want 0", got)
	}
}
