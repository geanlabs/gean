package node

import (
	"context"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/types"
)

func TestNextTickAt(t *testing.T) {
	const gt = 1700000000
	const gtMs = gt * 1000
	const iv = uint64(types.MillisecondsPerInterval)
	tests := []struct {
		name   string
		nowMs  uint64
		lastMs uint64
		want   uint64
	}{
		{"first_tick_before_genesis_waits_for_genesis", gtMs - 3000, 0, gtMs},
		{"first_tick_mid_interval_goes_to_next_boundary", gtMs + 10*iv + 790, 0, gtMs + 11*iv},
		{"woken_exactly_on_boundary_moves_a_full_interval", gtMs + 11*iv, gtMs + 11*iv, gtMs + 12*iv},
		{"normal_progression", gtMs + 11*iv + 3, gtMs + 11*iv, gtMs + 12*iv},
		// A wall clock stepped back behind the last delivered boundary must not
		// deliver that boundary, or any earlier one, a second time.
		{"clock_stepped_back_never_repeats_a_boundary", gtMs + 9*iv + 100, gtMs + 11*iv, gtMs + 12*iv},
		{"clock_stepped_forward_skips_ahead", gtMs + 20*iv + 5, gtMs + 11*iv, gtMs + 21*iv},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextTickAt(gt, tt.nowMs, tt.lastMs); got != tt.want {
				t.Fatalf("nextTickAt(%d, %d, %d) = %d, want %d", gt, tt.nowMs, tt.lastMs, got, tt.want)
			}
		})
	}
}

func TestNextTickAtFallsBackWhenGenesisOverflows(t *testing.T) {
	const nowMs = 1_700_000_000_000
	if got := nextTickAt(^uint64(0)/1000+1, nowMs, 0); got != nowMs+types.MillisecondsPerInterval {
		t.Fatalf("overflowing genesis: got %d, want a plain interval from now", got)
	}
}

// TestAlignedTicksLandOnIntervalBoundary checks the property the free-running
// ticker lacked: the tick arrives at the start of an interval measured from
// genesis, whatever moment the source was started at.
func TestAlignedTicksLandOnIntervalBoundary(t *testing.T) {
	genesis := uint64(time.Now().Unix()) - 10
	// Start the source partway into an interval, the position a node lands in
	// after an arbitrary restart.
	nowMs := uint64(time.Now().UnixMilli())
	next, _ := types.NextIntervalBoundaryMs(genesis, nowMs)
	if wait := time.Until(time.UnixMilli(int64(next))); wait > 0 {
		time.Sleep(wait + 300*time.Millisecond)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := alignedTicks(ctx, genesis)

	select {
	case tick := <-ticks:
		phaseMs := types.MillisIntoSlot(genesis, uint64(tick.UnixMilli())) % types.MillisecondsPerInterval
		// A tick released early would read just below a full interval.
		if phaseMs > 50 {
			t.Fatalf("tick landed %d ms into its interval; want it at the boundary", phaseMs)
		}
	case <-time.After(2 * types.MillisecondsPerInterval * time.Millisecond):
		t.Fatal("no tick within two intervals")
	}
}
