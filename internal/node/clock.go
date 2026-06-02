package node

import "github.com/geanlabs/gean/internal/types"

// currentSlot derives the current slot from a timestamp.
func (e *Engine) currentSlot(timestampMs uint64) uint64 {
	genesisMs := e.Store.Config().GenesisTime * 1000
	if timestampMs < genesisMs {
		return 0
	}
	return (timestampMs - genesisMs) / types.MillisecondsPerSlot
}

// currentInterval derives the current interval within a slot.
func (e *Engine) currentInterval(timestampMs uint64) uint64 {
	genesisMs := e.Store.Config().GenesisTime * 1000
	if timestampMs < genesisMs {
		return 0
	}
	totalIntervals := (timestampMs - genesisMs) / types.MillisecondsPerInterval
	return totalIntervals % types.IntervalsPerSlot
}
