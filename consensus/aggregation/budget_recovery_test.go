package aggregation

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/geanlabs/gean/crypto"
	"github.com/geanlabs/gean/metrics"
	"github.com/geanlabs/gean/shadow"
	"github.com/geanlabs/gean/storage/store"
	"github.com/geanlabs/gean/types"
)

// The signatures are only decoded: the injected prover tests scheduling and
// result retention, not cryptographic validity.
func budgetTestSnapshot() *Snapshot {
	snap := aggregateTestSnapshot(5, 6, 7)
	snap.headState.Validators = []*types.Validator{{Index: 0}, {Index: 1}, {Index: 2}}
	for _, dr := range [][32]byte{rootByte(2), rootByte(3)} {
		snap.attSigs[dr].Signatures = []store.AttestationSignatureEntry{{ValidatorID: 0}, {ValidatorID: 1}, {ValidatorID: 2}}
	}
	return snap
}

func TestAggregationBudgetRecovery(t *testing.T) {
	for _, tc := range []struct {
		name         string
		observations []time.Duration
		fail         bool
	}{
		{"first_slow_group", []time.Duration{2 * time.Second}, false},
		{"steady_then_spike", []time.Duration{1400 * time.Millisecond, 2500 * time.Millisecond}, false},
		{"failed_attempt", []time.Duration{2 * time.Second}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				e := newUnitCostEstimator()
				for _, d := range tc.observations {
					e.observeGroup(d, 0)
				}
				for session := 0; session < 5; session++ {
					before := e.nextGroupDuration()
					calls := 0
					prove := func(raw []crypto.RawSignature, children []crypto.Proof, root [32]byte, slot uint32) ([]byte, error) {
						calls++
						// All three of the group's signatures, not the two the
						// old per-signature budget floor allowed: the proof costs
						// the same either way.
						if slot != 6 || len(raw) != 3 || len(children) != 0 {
							t.Fatalf("unexpected inputs: slot=%d raw=%d children=%d", slot, len(raw), len(children))
						}
						time.Sleep(time.Second)
						if tc.fail {
							return nil, errors.New("test prover failed")
						}
						return []byte{1}, nil
					}
					aggs, payloads, deletes, truncated, skips := aggregateFromSnapshot(nil, budgetTestSnapshot(), time.Now().Add(SessionBudget), time.Now, MaxGroupsPerSession, shadow.Rates{}, e, prove, nil)
					if calls != 1 || !truncated || skips[metrics.AggGroupSkipBudget] != 1 || skips[metrics.AggGroupSkipTooFewSigners] != 1 {
						t.Fatalf("session=%d calls=%d truncated=%v skips=%v", session, calls, truncated, skips)
					}
					if tc.fail {
						if len(aggs) != 0 || len(payloads) != 0 || len(deletes) != 0 || skips[metrics.AggGroupSkipError] != 1 || e.nextGroupDuration() != before {
							t.Fatalf("failure changed results/estimate or lost skip: %v", skips)
						}
					} else {
						if len(aggs) != 1 || len(payloads) != 1 || len(deletes) != 3 {
							t.Fatalf("lost partial results: aggs=%d payloads=%d deletes=%d", len(aggs), len(payloads), len(deletes))
						}
						if e.nextGroupDuration() >= before {
							t.Fatal("successful attempt did not recalibrate")
						}
						// Every signature the group held is covered and retired:
						// the proof costs the same whether it carries two or
						// three, so none is left behind for a later session.
						if types.BitlistCount(aggs[0].Proof.Participants) != 3 ||
							deletes[0].ValidatorID != 0 || deletes[1].ValidatorID != 1 || deletes[2].ValidatorID != 2 {
							t.Fatal("group did not cover and retire every signature it held")
						}
					}
				}
			})
		})
	}
}

func TestAggregationBudgetDeadline(t *testing.T) {
	for _, tc := range []struct {
		name          string
		deadline      time.Duration
		proofTime     time.Duration
		wantCalls     int
		wantTruncated bool
	}{
		{"expired", -time.Second, 0, 0, true},
		{"at_deadline", 0, 0, 0, true},
		{"first_proof_overruns", SessionBudget, 2 * time.Second, 1, true},
		{"enough_for_later_group", SessionBudget, 100 * time.Millisecond, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				prove := func([]crypto.RawSignature, []crypto.Proof, [32]byte, uint32) ([]byte, error) {
					calls++
					time.Sleep(tc.proofTime)
					return []byte{1}, nil
				}
				aggs, _, _, truncated, skips := aggregateFromSnapshot(nil, budgetTestSnapshot(), time.Now().Add(tc.deadline), time.Now, MaxGroupsPerSession, shadow.Rates{}, newUnitCostEstimator(), prove, nil)
				if calls != tc.wantCalls || len(aggs) != calls || truncated != tc.wantTruncated {
					t.Fatalf("calls=%d aggs=%d truncated=%v skips=%v", calls, len(aggs), truncated, skips)
				}
			})
		})
	}
}

// The estimator must learn on the session clock the deadline uses. With a
// clock that does not advance while proving, as in a deterministic
// simulation, learning from wall time would compare real proving cost with a
// window that never shrinks, so how many groups a session proves would depend
// on how fast the host is.
func TestAggregationBudgetUsesSessionClock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		frozen := time.Now()
		calls := 0
		prove := func([]crypto.RawSignature, []crypto.Proof, [32]byte, uint32) ([]byte, error) {
			calls++
			time.Sleep(2 * time.Second) // wall time passes; the session clock does not
			return []byte{1}, nil
		}
		e := newUnitCostEstimator()
		before := e.nextGroupDuration()
		_, _, _, truncated, skips := aggregateFromSnapshot(nil, budgetTestSnapshot(), frozen.Add(SessionBudget), func() time.Time { return frozen }, MaxGroupsPerSession, shadow.Rates{}, e, prove, nil)
		if calls != 2 || truncated {
			t.Fatalf("calls=%d truncated=%v skips=%v, want both groups proven", calls, truncated, skips)
		}
		if e.nextGroupDuration() != before {
			t.Fatal("estimator learned from wall time instead of the session clock")
		}
	})
}
