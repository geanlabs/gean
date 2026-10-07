package node

import (
	"time"

	"github.com/geanlabs/gean/types"
)

// Clock is the engine's source of consensus time: slot and interval
// boundaries, aggregation deadlines and sync status are all read from it, so a
// simulation can control time by supplying its own. Durations measured only
// for metrics use the wall clock directly.
type Clock interface {
	Now() time.Time
}

// SystemClock reads the wall clock.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

func (e *Engine) nowMs() uint64 {
	return uint64(e.clock.Now().UnixMilli())
}

func (e *Engine) currentSlot(timestampMs uint64) uint64 {
	if e == nil || e.store == nil {
		return 0
	}
	return types.CurrentSlot(e.store.Config().GenesisTime, timestampMs)
}

func (e *Engine) currentInterval(timestampMs uint64) uint64 {
	if e == nil || e.store == nil {
		return 0
	}
	return types.CurrentInterval(e.store.Config().GenesisTime, timestampMs)
}

func (e *Engine) millisIntoSlot(timestampMs uint64) uint64 {
	if e == nil || e.store == nil {
		return 0
	}
	return types.MillisIntoSlot(e.store.Config().GenesisTime, timestampMs)
}
