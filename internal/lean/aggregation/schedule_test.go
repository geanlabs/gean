package aggregation

import (
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/core/clock"
	"github.com/geanlabs/gean/internal/core/storage"
	"github.com/geanlabs/gean/internal/lean/store"
	"github.com/geanlabs/gean/internal/lean/types"
)

// The session must finish before interval 4, when new payloads are promoted and
// gossiped. Measuring a fixed span from the worker's start gives the
// early-interval-1 path a deadline short of that boundary, surrendering most of
// the head start that path exists to create. Anchoring to the slot lands both
// dispatch paths on the same instant.
func TestAggregationDeadlineAnchorsToIntervalFour(t *testing.T) {
	agg := makeTestAggregator()
	genesisMs := agg.cfg.Store.Config().GenesisTime * 1000
	slotStart := genesisMs + types.MillisecondsPerSlot*7

	const interval = types.MillisecondsPerInterval

	boundary := time.UnixMilli(int64(slotStart + 4*interval))

	for _, tc := range []struct {
		name       string
		intoSlot   uint64
		wantWindow time.Duration
	}{
		{"early interval 1", interval, 3 * interval * time.Millisecond},
		{"interval 2 fallback", 2 * interval, 2 * interval * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := agg.deadline(slotStart + tc.intoSlot)
			if !got.Equal(boundary) {
				t.Fatalf("deadline = %v, want the interval-4 boundary %v", got, boundary)
			}
			if window := got.Sub(time.UnixMilli(int64(slotStart + tc.intoSlot))); window != tc.wantWindow {
				t.Fatalf("window = %v, want %v", window, tc.wantWindow)
			}
		})
	}

	// A dispatch past the boundary would otherwise hand the worker an expired
	// deadline, which it reads as "stop before the first group" — the produced=0
	// shape this branch exists to remove.
	late := agg.deadline(slotStart + 4*interval)
	if window := late.Sub(time.UnixMilli(int64(slotStart + 4*interval))); window <= 0 {
		t.Fatalf("late dispatch window = %v, want a usable window", window)
	}
}

// An aggregator only receives the subnets it joined, so measuring the early
// trigger's quorum against the whole registry makes it unreachable for anything
// but a node covering every subnet — the early path then never fires and the
// head start it exists to give is never taken.
func TestExpectedVotersPerSlotFollowsSubscribedSubnets(t *testing.T) {
	for _, tc := range []struct {
		name       string
		validators uint64
		committees uint64
		subnets    []uint64
		want       uint64
	}{
		{name: "single committee", validators: 12, committees: 1, want: 12},
		{name: "all subnets subscribed", validators: 12, committees: 4, subnets: []uint64{0, 1, 2, 3}, want: 12},
		{name: "no subnets configured", validators: 12, committees: 4, want: 12},
		{name: "one of four", validators: 12, committees: 4, subnets: []uint64{2}, want: 3},
		{name: "two of four", validators: 12, committees: 4, subnets: []uint64{0, 3}, want: 6},
		{name: "uneven registry", validators: 10, committees: 4, subnets: []uint64{0}, want: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agg := &Aggregator{
				numValidators: tc.validators,
				cfg:           Config{Subnets: func() (uint64, []uint64) { return tc.committees, tc.subnets }},
			}
			if got := agg.expectedVotersPerSlot(); got != tc.want {
				t.Fatalf("voters=%d, want %d", got, tc.want)
			}
		})
	}

	// The quorum a single-subnet aggregator must reach has to be reachable from
	// the votes it can actually receive.
	agg := &Aggregator{numValidators: 12, cfg: Config{Subnets: func() (uint64, []uint64) { return 4, []uint64{1} }}}
	if q := earlyAggregationQuorum(agg.expectedVotersPerSlot()); uint64(q) > agg.expectedVotersPerSlot() {
		t.Fatalf("quorum %d exceeds the %d votes this node can receive", q, agg.expectedVotersPerSlot())
	}
}

// makeTestAggregator returns an aggregator over a store whose genesis time is
// 1000.
func makeTestAggregator() *Aggregator {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	s.SetConfig(&types.ChainConfig{GenesisTime: 1000})
	return New(Config{Store: s, Clock: clock.Real{}})
}
