package node

import (
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/aggregation"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

const testSlot = 7

// backlogTestEngine is an aggregator with one pooled vote and a head state of
// four validators; validator 3 is ours, so it proposes slot 7 (7 mod 4).
func backlogTestEngine(t *testing.T) (*Engine, uint64) {
	t.Helper()
	e := makeTestEngine()
	head := e.Store.Head()
	state := e.Store.GetState(head)
	state.Validators = make([]*types.Validator, 4)
	for i := range state.Validators {
		state.Validators[i] = &types.Validator{Index: uint64(i)}
	}
	e.Store.InsertState(head, state)
	e.Store.AttestationSignatures.Insert([32]byte{0xaa}, &types.AttestationData{Slot: testSlot - 1}, 0, [types.SignatureSize]byte{})
	slotStart := e.Store.Config().GenesisTime*1000 + types.MillisecondsPerSlot*testSlot
	return e, slotStart
}

func takeDispatch(t *testing.T, e *Engine) (aggregation.Dispatch, bool) {
	t.Helper()
	select {
	case d := <-e.AggregationDispatchCh:
		return d, true
	default:
		return aggregation.Dispatch{}, false
	}
}

// Each backlog window ends inside its slot, where the next piece of slot work
// needs the prover.
func TestBacklogDispatchWindows(t *testing.T) {
	const interval = types.MillisecondsPerInterval
	for _, tc := range []struct {
		name     string
		interval uint64
		endsAt   uint64 // ms into the slot
		early    bool
	}{
		{"interval 0 ends at interval 2", 0, 2 * interval, true},
		{"interval 3 ends at interval 4", 3, 4 * interval, false},
		{"interval 4 ends with the slot", 4, types.MillisecondsPerSlot, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, slotStart := backlogTestEngine(t)
			e.dispatchBacklog(slotStart+tc.interval*interval, testSlot, tc.interval, true)
			d, ok := takeDispatch(t, e)
			if !ok {
				t.Fatal("no backlog dispatch")
			}
			if !d.Backlog || d.Slot != testSlot || d.Early != tc.early {
				t.Fatalf("dispatch backlog=%v slot=%d early=%v", d.Backlog, d.Slot, d.Early)
			}
			if want := time.UnixMilli(int64(slotStart + tc.endsAt)); !d.Deadline.Equal(want) {
				t.Fatalf("deadline = %v, want %v", d.Deadline, want)
			}
		})
	}
}

// Interval 1 is where the early aggregation path fires on this slot's votes,
// and interval 2 belongs to the slot's own session. A non-aggregator, no pacer
// or an empty pool get nothing either, before any snapshot is built on the
// dispatch loop. When the pacer itself is closed is covered by its own tests.
func TestBacklogDispatchNotOffered(t *testing.T) {
	for _, tc := range []struct {
		name     string
		interval uint64
		isAgg    bool
		engine   func(*testing.T) (*Engine, uint64)
	}{
		{"interval 1", 1, true, backlogTestEngine},
		{"interval 2", 2, true, backlogTestEngine},
		{"not an aggregator", 3, false, backlogTestEngine},
		{"no pacer", 3, true, func(t *testing.T) (*Engine, uint64) {
			e, slotStart := backlogTestEngine(t)
			e.AggregationPacer = nil
			return e, slotStart
		}},
		{"empty pool", 3, true, func(*testing.T) (*Engine, uint64) {
			e := makeTestEngine()
			return e, e.Store.Config().GenesisTime*1000 + types.MillisecondsPerSlot*testSlot
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, slotStart := tc.engine(t)
			e.dispatchBacklog(slotStart+tc.interval*types.MillisecondsPerInterval, testSlot, tc.interval, tc.isAgg)
			if _, ok := takeDispatch(t, e); ok {
				t.Fatal("backlog dispatched")
			}
		})
	}
}

// The block needs the prover at interval 0 of a slot this node proposes, so
// backlog work stays out of that interval and out of the end of the slot
// before, where the interval-2 session already stops early.
func TestBacklogDispatchAroundOwnProposal(t *testing.T) {
	const interval = types.MillisecondsPerInterval
	e, slotStart := backlogTestEngine(t)
	e.Keys = xmss.NewKeyManager(map[uint64]*xmss.ValidatorKeyPair{3: nil}, nil)
	prevSlotStart := slotStart - types.MillisecondsPerSlot

	for _, tc := range []struct {
		slot, start, interval uint64
	}{
		{testSlot, slotStart, 0},
		{testSlot - 1, prevSlotStart, 3},
		{testSlot - 1, prevSlotStart, 4},
	} {
		e.dispatchBacklog(tc.start+tc.interval*interval, tc.slot, tc.interval, true)
		if _, ok := takeDispatch(t, e); ok {
			t.Fatalf("backlog dispatched at slot %d interval %d around our proposal", tc.slot, tc.interval)
		}
	}

	// After the proposal slot's interval 2 the prover is free again.
	e.dispatchBacklog(slotStart+3*interval, testSlot, 3, true)
	if _, ok := takeDispatch(t, e); !ok {
		t.Fatal("backlog withheld after our proposal")
	}
}

// The interval-2 session used to drop to one group in the slot before this node
// proposes; it now ends an interval early instead, leaving room for one
// overrunning proof before the block needs the prover.
func TestIntervalTwoSessionEndsEarlyBeforeOwnProposal(t *testing.T) {
	const interval = types.MillisecondsPerInterval
	e, slotStart := backlogTestEngine(t)
	e.Keys = xmss.NewKeyManager(map[uint64]*xmss.ValidatorKeyPair{3: nil}, nil)
	prevSlotStart := slotStart - types.MillisecondsPerSlot

	e.dispatchAggregationCycle(prevSlotStart+2*interval, testSlot-1, true)
	d, ok := takeDispatch(t, e)
	if !ok {
		t.Fatal("no interval-2 dispatch")
	}
	if want := time.UnixMilli(int64(prevSlotStart + 3*interval)); !d.Deadline.Equal(want) {
		t.Fatalf("deadline = %v, want interval 3 %v", d.Deadline, want)
	}
	if d.Backlog {
		t.Fatal("interval-2 session marked as backlog")
	}
}

func TestAggregationHeadStateDecodesOncePerHead(t *testing.T) {
	e, _ := backlogTestEngine(t)
	first := e.aggregationHeadState()
	if first == nil || e.aggregationHeadState() != first {
		t.Fatal("head state decoded again for an unchanged head")
	}
	other := [32]byte{0x02}
	e.Store.InsertState(other, &types.State{Config: &types.ChainConfig{}, LatestBlockHeader: &types.BlockHeader{}})
	e.Store.SetHead(other)
	if e.aggregationHeadState() == first {
		t.Fatal("cached state kept after the head moved")
	}
}

func TestTickLateness(t *testing.T) {
	interval := types.MillisecondsPerInterval * time.Millisecond
	for _, tc := range []struct {
		name  string
		phase uint64
		gap   time.Duration
		want  time.Duration
	}{
		{"on time", 3, interval, 3 * time.Millisecond},
		{"late within the interval", 250, interval + 250*time.Millisecond, 250 * time.Millisecond},
		// Handled 1.1s late: the phase has wrapped to 300ms, the gap has not.
		{"late past the interval", 300, interval + 1100*time.Millisecond, 1100 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tickLateness(tc.phase, tc.gap); got != tc.want {
				t.Fatalf("lateness = %v, want %v", got, tc.want)
			}
		})
	}
}
