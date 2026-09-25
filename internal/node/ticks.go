package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/internal/types"
)

// alignedTicks delivers one tick per interval boundary, anchored to genesis.
//
// A time.Ticker keeps the period but not the phase: started at an arbitrary
// moment, it fires at boundary+φ for the life of the process, so every
// interval's duties run φ late and a node attests on a later head than the
// rest of the network. Here each deadline is re-derived from the wall clock,
// so the phase is right from the first tick and stays right across clock steps.
//
// Each boundary is delivered at most once and never before the wall clock has
// reached it: onTick keeps no per-interval guard, so an early or repeated tick
// would re-run the previous interval's duties.
//
// Delivery is a non-blocking send into a one-slot buffer, as with time.Ticker.
// A boundary that passes while the dispatch loop is busy is handled as soon as
// it frees; later ones are dropped rather than queued stale.
func alignedTicks(ctx context.Context, genesisTime uint64) <-chan time.Time {
	ch := make(chan time.Time, 1)
	go func() {
		timer := time.NewTimer(time.Hour)
		defer timer.Stop()
		var lastMs uint64
		for {
			targetMs := nextTickAt(genesisTime, uint64(time.Now().UnixMilli()), lastMs)
			// Timers measure a duration on the monotonic clock; the boundary is
			// on the wall clock. Re-check after every wake so a slewed or stepped
			// wall clock can never release a tick early.
			for {
				wait := time.Until(time.UnixMilli(int64(targetMs)))
				if wait <= 0 {
					break
				}
				timer.Reset(wait)
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
				}
			}
			lastMs = targetMs
			select {
			case ch <- time.Now():
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
// unrepresentable it falls back to a plain interval from now, the old
// free-running behaviour, rather than stopping the clock.
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
