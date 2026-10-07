package aggregation

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/proving"
	"github.com/geanlabs/gean/internal/shadow"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

type proveFunc = func([]xmss.CPubKey, []xmss.CSig, []xmss.ChildProof, [32]byte, uint32) ([]byte, error)

func testWorker(t *testing.T, prove proveFunc) *worker {
	w, _ := testWorkerWithQueue(t, prove)
	return w
}

// testWorkerWithQueue also returns the send side of the worker's dispatch queue.
func testWorkerWithQueue(t *testing.T, prove proveFunc) (*worker, chan Dispatch) {
	t.Helper()
	cache := xmss.NewPubKeyCache()
	t.Cleanup(cache.Close)
	queue := make(chan Dispatch, 1)
	return &worker{
		dispatches: queue,
		store:      store.NewConsensusStore(storage.NewInMemoryBackend()),
		cache:      cache,
		gate:       proving.NewGate(),
		pacer:      NewPacer(),
		estimator:  newUnitCostEstimator(),
		provedAt:   make(map[[32]byte]time.Time),
		prove:      prove,
	}, queue
}

func countingProver(calls *[]uint32, cost time.Duration) proveFunc {
	return func(_ []xmss.CPubKey, _ []xmss.CSig, _ []xmss.ChildProof, _ [32]byte, slot uint32) ([]byte, error) {
		*calls = append(*calls, slot)
		time.Sleep(cost)
		return []byte{1}, nil
	}
}

// With the cap of two gone, a session proves every group its deadline has room
// for. On a fast host that is more than two; time, not a count, is the bound.
func TestSessionProvesEveryGroupItsDeadlineAllows(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		snap := aggregateTestSnapshot(5, 6, 7, 8)
		snap.headState.Validators = []*types.Validator{{Index: 0}, {Index: 1}}
		for i := range 4 {
			snap.attSigs[rootByte(byte(i+1))].Signatures = []store.AttestationSignatureEntry{{ValidatorID: 0}, {ValidatorID: 1}}
		}
		cache := xmss.NewPubKeyCache()
		defer cache.Close()
		var calls []uint32
		aggs, _, _, truncated, skips := aggregateFromSnapshotWithProver(nil, snap, cache,
			time.Now().Add(SessionBudget), shadow.Rates{}, newUnitCostEstimator(), countingProver(&calls, 300*time.Millisecond))
		if len(calls) != 4 || len(aggs) != 4 || truncated {
			t.Fatalf("calls=%d aggs=%d truncated=%v skips=%v, want all four groups proved", len(calls), len(aggs), truncated, skips)
		}
	})
}

// A backlog session with no budget must not take the prover at all, and must
// close the slot so the dispatch loop stops building snapshots for it.
func TestBacklogSessionWaitsForBudget(t *testing.T) {
	var calls []uint32
	w := testWorker(t, countingProver(&calls, 0))
	w.session(context.Background(), Dispatch{Snapshot: budgetTestSnapshot(), Slot: 9, Deadline: time.Now().Add(time.Hour), Backlog: true})
	if len(calls) != 0 {
		t.Fatalf("proved %d groups with no budget", len(calls))
	}
	if w.pacer.BacklogOpen(9, false) {
		t.Fatal("slot left open after the budget refused work")
	}
	if !w.pacer.limited {
		t.Fatal("refusal not counted toward growing the budget")
	}
	if !w.gate.Acquire(context.Background(), false) {
		t.Fatal("prover not released")
	}
}

// A proof cannot be stopped once started, so a backlog session with less budget
// left than one proof costs must not start: the first proof of an interval-2
// session always runs, but a backlog one would run into the window the
// interval-2 session or a proposal needs.
func TestBacklogSessionStartsOnlyWhenAProofFits(t *testing.T) {
	var calls []uint32
	w := testWorker(t, countingProver(&calls, 0))
	w.pacer.budget = w.estimator.nextGroupDuration() - time.Millisecond
	w.session(context.Background(), Dispatch{Snapshot: budgetTestSnapshot(), Slot: 9, Deadline: time.Now().Add(time.Hour), Backlog: true})
	if len(calls) != 0 {
		t.Fatalf("started %d proofs with less budget than one costs", len(calls))
	}
	if w.pacer.BacklogOpen(9, false) {
		t.Fatal("slot left open after the budget refused work")
	}
}

// The budget, not the dispatch deadline, bounds a backlog session: it stops
// once the next proof no longer fits what is left of the slot's budget.
func TestBacklogSessionStaysWithinBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls []uint32
		w := testWorker(t, countingProver(&calls, 600*time.Millisecond))
		w.pacer.budget = time.Second
		w.session(context.Background(), Dispatch{Snapshot: budgetTestSnapshot(), Slot: 9, Deadline: time.Now().Add(time.Hour), Backlog: true})
		if len(calls) != 1 {
			t.Fatalf("proved %d groups, want 1 within a 1s budget at 600ms each", len(calls))
		}
		if w.pacer.spent != 600*time.Millisecond {
			t.Fatalf("charged %v, want the session's 600ms", w.pacer.spent)
		}
		if w.pacer.BacklogOpen(9, false) {
			t.Fatal("slot left open after the budget stopped the session")
		}
	})
}

// The interval-2 dispatch is queued while a backlog session proves; the backlog
// session must hand over at the next proof boundary and leave the dispatch for
// the worker loop.
func TestBacklogSessionGivesWayToNextDispatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls []uint32
		var queue chan Dispatch
		var w *worker
		w, queue = testWorkerWithQueue(t, func(_ []xmss.CPubKey, _ []xmss.CSig, _ []xmss.ChildProof, _ [32]byte, slot uint32) ([]byte, error) {
			calls = append(calls, slot)
			queue <- Dispatch{Slot: 10}
			time.Sleep(100 * time.Millisecond)
			return []byte{1}, nil
		})
		w.pacer.budget = backlogBudgetMax
		w.session(context.Background(), Dispatch{Snapshot: budgetTestSnapshot(), Slot: 9, Deadline: time.Now().Add(time.Hour), Backlog: true})
		if len(calls) != 1 {
			t.Fatalf("proved %d groups, want 1 before handing over", len(calls))
		}
		if d := <-queue; d.Slot != 10 {
			t.Fatalf("queued dispatch slot=%d, want 10", d.Slot)
		}
	})
}

// Groups a session leaves behind are reported under why it stopped: a waiting
// proposal for any session, the next dispatch only for a backlog one. The
// interval-2 session never gives way to a queued dispatch.
func TestYieldReasons(t *testing.T) {
	w, queue := testWorkerWithQueue(t, nil)
	if r := w.yieldFor(true)(); r != "" {
		t.Fatalf("idle yield = %q, want none", r)
	}
	queue <- Dispatch{}
	if r := w.yieldFor(true)(); r != metrics.AggGroupSkipSuperseded {
		t.Fatalf("backlog yield = %q, want %q", r, metrics.AggGroupSkipSuperseded)
	}
	if r := w.yieldFor(false)(); r != "" {
		t.Fatalf("interval-2 session yielded to a queued dispatch: %q", r)
	}
	if !w.gate.Acquire(context.Background(), true) {
		t.Fatal("proposal acquire failed")
	}
	for _, backlog := range []bool{true, false} {
		if r := w.yieldFor(backlog)(); r != metrics.AggGroupSkipProposalPending {
			t.Fatalf("backlog=%v yield = %q, want %q", backlog, r, metrics.AggGroupSkipProposalPending)
		}
	}
}

// Before interval 2 this slot's votes are still arriving; a backlog session
// then proves only older groups.
func TestBacklogSessionLeavesCurrentSlotVotes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls []uint32
		w := testWorker(t, countingProver(&calls, 100*time.Millisecond))
		w.pacer.budget = backlogBudgetMax
		// budgetTestSnapshot holds provable groups for slots 6 and 7.
		snap := budgetTestSnapshot()
		snap.slot = 7
		w.session(context.Background(), Dispatch{Snapshot: snap, Slot: 7, Deadline: time.Now().Add(time.Hour), Backlog: true, Early: true})
		if len(calls) != 1 || calls[0] != 6 {
			t.Fatalf("proved slots %v, want only the earlier slot 6", calls)
		}
	})
}

// A snapshot can carry signatures a proof finished after it was taken already
// consumed; proving them again spends a proof on no new coverage.
func TestSessionSkipsRootsProvedSinceItsSnapshot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls []uint32
		w := testWorker(t, countingProver(&calls, 100*time.Millisecond))
		snap := budgetTestSnapshot()
		snap.takenAt = time.Now()
		w.provedAt[rootByte(2)] = snap.takenAt.Add(time.Millisecond)  // after: skip
		w.provedAt[rootByte(3)] = snap.takenAt.Add(-time.Millisecond) // before: keep
		w.session(context.Background(), Dispatch{Snapshot: snap, Slot: 7, Deadline: time.Now().Add(time.Hour)})
		if len(calls) != 1 || calls[0] != 7 {
			t.Fatalf("proved slots %v, want only slot 7's root", calls)
		}
		if !w.provedAt[rootByte(3)].After(snap.takenAt) {
			t.Fatal("proved root not recorded")
		}
	})
}

// A backlog session that went through every group has nothing more for its
// window; offering it another snapshot would rebuild one on the dispatch loop
// every interval of a stall. The other window of the slot stays open: votes the
// interval-2 session could not finish arrive after an early window has run.
// Running out of work is not demand, so the budget does not grow from it.
func TestBacklogSessionClosesItsWindowWhenOutOfWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls []uint32
		w := testWorker(t, countingProver(&calls, 100*time.Millisecond))
		w.pacer.budget = backlogBudgetMax
		w.session(context.Background(), Dispatch{Snapshot: budgetTestSnapshot(), Slot: 9, Deadline: time.Now().Add(time.Hour), Backlog: true, Early: true})
		if len(calls) != 2 {
			t.Fatalf("proved %d groups, want both provable groups", len(calls))
		}
		if w.pacer.BacklogOpen(9, true) {
			t.Fatal("window left open with no work left")
		}
		if !w.pacer.BacklogOpen(9, false) {
			t.Fatal("finishing the early window closed the late one")
		}
		if w.pacer.limited {
			t.Fatal("running out of work counted as demand")
		}
	})
}

// A window shorter than one proof (an 800ms window on a host where proofs take
// longer) is closed without counting as demand, so the dispatch loop stops
// building snapshots for it.
func TestBacklogSessionClosesAWindowTooShortForAProof(t *testing.T) {
	var calls []uint32
	w := testWorker(t, countingProver(&calls, 0))
	w.pacer.budget = backlogBudgetMax
	w.session(context.Background(), Dispatch{Snapshot: budgetTestSnapshot(), Slot: 9, Deadline: time.Now().Add(time.Millisecond), Backlog: true})
	if len(calls) != 0 {
		t.Fatal("started a proof in a window too short for one")
	}
	if w.pacer.BacklogOpen(9, false) {
		t.Fatal("too-short window left open")
	}
	if !w.pacer.BacklogOpen(9, true) || w.pacer.limited {
		t.Fatal("a short window spent the slot's budget")
	}
}

// A child proof costs several times a raw-only proof, and the interval-2
// session admits the first child whatever time is left. A backlog session must
// not: a backlog proof that runs into interval 2 starves the slot's own votes.
// Without room for the child, the group proves what its raw signatures cover.
func TestBacklogSessionPricesChildProofs(t *testing.T) {
	// Seeds: a raw-only proof 300ms, a child 1.5s more.
	for _, tc := range []struct {
		name         string
		backlog      bool
		window       time.Duration
		wantChildren int
	}{
		{"backlog: no room for the child", true, time.Second, 0},
		{"backlog: room for the child but not the proof around it", true, 1700 * time.Millisecond, 0},
		{"backlog: room for both", true, 2500 * time.Millisecond, 1},
		{"interval 2: first child admitted regardless", false, time.Second, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var children []int
				w := testWorker(t, func(_ []xmss.CPubKey, _ []xmss.CSig, ch []xmss.ChildProof, _ [32]byte, _ uint32) ([]byte, error) {
					children = append(children, len(ch))
					return []byte{1}, nil
				})
				// The window, not the budget, decides here.
				w.pacer.budget = time.Hour
				snap := childTestSnapshot()
				w.session(context.Background(), Dispatch{Snapshot: snap, Slot: 9, Deadline: time.Now().Add(tc.window), Backlog: tc.backlog})
				if len(children) != 1 || children[0] != tc.wantChildren {
					t.Fatalf("proofs with children %v, want one proof with %d", children, tc.wantChildren)
				}
			})
		})
	}
}

// childTestSnapshot holds one group whose raw signatures cover validators 0
// and 1, plus an existing aggregate covering validator 2 that only a child
// proof can add.
func childTestSnapshot() *Snapshot {
	snap := aggregateTestSnapshot(6)
	snap.headState.Validators = []*types.Validator{{Index: 0}, {Index: 1}, {Index: 2}}
	snap.attSigs[rootByte(1)].Signatures = []store.AttestationSignatureEntry{{ValidatorID: 0}, {ValidatorID: 1}}
	snap.newEntries[rootByte(1)] = &store.PayloadEntry{
		Data:   snap.attSigs[rootByte(1)].Data,
		Proofs: []*types.SingleMessageAggregate{{Participants: types.BitlistFromIndices([]uint64{2}), Proof: []byte{1}}},
	}
	return snap
}

func TestWorkerForgetsOldProvedRoots(t *testing.T) {
	w := testWorker(t, nil)
	w.provedAt[rootByte(1)] = time.Now().Add(-provedAtRetention - time.Second)
	w.provedAt[rootByte(2)] = time.Now()
	w.recordProved(nil)
	if _, ok := w.provedAt[rootByte(1)]; ok {
		t.Fatal("expired root kept")
	}
	if _, ok := w.provedAt[rootByte(2)]; !ok {
		t.Fatal("recent root dropped")
	}
}
