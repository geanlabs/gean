package node

import "github.com/geanlabs/gean/internal/types"

func (e *Engine) currentSlot(timestampMs uint64) uint64 {
	if e == nil || e.Store == nil {
		return 0
	}
	return types.CurrentSlot(e.Store.Config().GenesisTime, timestampMs)
}

func (e *Engine) currentInterval(timestampMs uint64) uint64 {
	if e == nil || e.Store == nil {
		return 0
	}
	return types.CurrentInterval(e.Store.Config().GenesisTime, timestampMs)
}

// claimInterval reports whether the interval containing timestampMs has not
// had its duties run yet, and marks it as run. When the dispatch loop falls
// behind, two ticks can be handled inside one interval; running its duties a
// second time would sign a second attestation for the slot. Before genesis
// millisIntoSlot is 0, so each pre-genesis tick claims its own timestamp and
// none can shadow the genesis interval.
func (e *Engine) claimInterval(timestampMs uint64) bool {
	start := timestampMs - e.millisIntoSlot(timestampMs)%types.MillisecondsPerInterval
	if start <= e.lastIntervalStartMs {
		return false
	}
	e.lastIntervalStartMs = start
	return true
}

func (e *Engine) millisIntoSlot(timestampMs uint64) uint64 {
	if e == nil || e.Store == nil {
		return 0
	}
	return types.MillisIntoSlot(e.Store.Config().GenesisTime, timestampMs)
}
