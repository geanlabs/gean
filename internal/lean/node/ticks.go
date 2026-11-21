package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/internal/core/clock"
	"github.com/geanlabs/gean/internal/lean/types"
)

// alignedTicks delivers one tick per interval boundary, measured from genesis.
// A time.Ticker would keep the period but take its phase from process start.
//
// Each boundary is delivered at most once and never before the wall clock
// reaches it. Delivery is a non-blocking send into a one-slot buffer, as with
// time.Ticker: a tick missed while dispatch is busy is dropped, not queued.
// A tick handled late can still land in an interval already run; onTick's
// claimInterval drops it.
//
// Time comes from clk, so a test can drive ticks with a clock.Fake.
func alignedTicks(ctx context.Context, clk clock.Clock, genesisTime uint64) <-chan time.Time {
	ch := make(chan time.Time, 1)
	go func() {
		timer := clk.NewTimer(time.Hour)
		defer timer.Stop()
		var lastMs uint64
		for {
			targetMs := nextTickAt(genesisTime, clock.UnixMilli(clk), lastMs)
			// Timers measure a duration on the monotonic clock; the boundary is
			// on the wall clock. Re-check after every wake so a slewed or stepped
			// wall clock can never release a tick early.
			for {
				wait := clock.FromUnixMilli(targetMs).Sub(clk.Now())
				if wait <= 0 {
					break
				}
				timer.Reset(wait)
				select {
				case <-ctx.Done():
					return
				case <-timer.C():
				}
			}
			lastMs = targetMs
			select {
			case ch <- clk.Now():
			default:
			}
		}
	}()
	return ch
}

// nextTickAt returns the wall-clock millisecond of the next tick to deliver:
// the first interval boundary after nowMs, and never one at or before lastMs,
// the boundary delivered previously (zero before the first). Holding to lastMs
// keeps delivery monotonic when the wall clock steps backwards. If genesis is
// unrepresentable it falls back to a plain interval from now rather than
// stopping the clock.
func nextTickAt(genesisTime, nowMs, lastMs uint64) uint64 {
	target, ok := types.NextIntervalBoundaryMs(genesisTime, nowMs)
	if !ok {
		return nowMs + types.MillisecondsPerInterval
	}
	if lastMs != 0 && target <= lastMs {
		return lastMs + types.MillisecondsPerInterval
	}
	return target
}
