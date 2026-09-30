package aggregation

import (
	"strings"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/shadow"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

// A target whose own state was never stored must no longer cost its group.
// The validator registry is written once at genesis and never by the state
// transition, so the head state resolves the same signers — this is the case
// that silently produced nothing for 355 consecutive slots on devnet-5.
func TestAggregateResolvesSignersWithoutTargetState(t *testing.T) {
	target := &types.Checkpoint{Slot: 42, Root: rootByte(9)}
	snap := &Snapshot{
		headState: &types.State{
			LatestFinalized: &types.Checkpoint{Slot: 0},
			Validators:      make([]*types.Validator, 8),
		},
		attSigs: map[[32]byte]*store.AttestationDataEntry{
			rootByte(1): {Data: &types.AttestationData{Slot: 42, Target: target}},
			rootByte(2): {Data: &types.AttestationData{Slot: 43, Target: target}},
		},
	}

	_, _, _, _, skips := aggregateFromSnapshot(nil, snap, xmss.NewPubKeyCache(), time.Time{}, MaxGroupsPerSession, shadow.Rates{}, newUnitCostEstimator())

	// No group may be dropped for a reason that no longer exists; these groups
	// carry no signatures, so they fall out as too-few-signers instead.
	if got := skips[metrics.AggGroupSkipTooFewSigners]; got != 2 {
		t.Errorf("too_few_signers=%d, want 2 (groups reached signer selection)", got)
	}
	for reason := range skips {
		if reason == "missing_target_state" {
			t.Errorf("group dropped for a missing target state; the head registry should have been used")
		}
	}
}

// The summary is what reaches the operator's log line, so it must name every
// non-zero reason and stay stable across runs despite map iteration order.
func TestGroupSkipsSummary(t *testing.T) {
	if got := (groupSkips{}).summary(); got != "" {
		t.Errorf("empty summary=%q, want \"\"", got)
	}

	skips := groupSkips{
		metrics.AggGroupSkipTooFewSigners: 2,
		metrics.AggGroupSkipSessionCap:    5,
	}
	got := skips.summary()

	for _, want := range []string{"session_cap=5", "too_few_signers=2"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q missing %q", got, want)
		}
	}
	for range 5 {
		if again := skips.summary(); again != got {
			t.Fatalf("summary unstable: %q then %q", got, again)
		}
	}
}
