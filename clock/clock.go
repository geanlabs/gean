// Package clock provides consensus time tracking for the Lean protocol.
//
// SlotClock is not thread-safe. The caller (typically forkchoice.Store)
// must provide synchronization when accessing the clock from multiple goroutines.
package clock

import "github.com/devylongs/gean/types"

// SlotClock tracks consensus time as interval ticks since genesis.
type SlotClock struct {
	intervals   uint64
	genesisTime uint64
}

// New creates a SlotClock initialized to the start of the given slot.
func New(genesisTime uint64, startSlot types.Slot) *SlotClock {
	return &SlotClock{
		intervals:   uint64(startSlot) * types.IntervalsPerSlot,
		genesisTime: genesisTime,
	}
}

// Intervals returns the total number of intervals elapsed since genesis.
func (c *SlotClock) Intervals() uint64 { return c.intervals }

// CurrentSlot returns the slot corresponding to the current interval count.
func (c *SlotClock) CurrentSlot() types.Slot {
	return types.Slot(c.intervals / types.IntervalsPerSlot)
}

// CurrentInterval returns the interval within the current slot (0 to IntervalsPerSlot-1).
func (c *SlotClock) CurrentInterval() uint64 {
	return c.intervals % types.IntervalsPerSlot
}

// Tick advances the clock by one interval and returns the new interval within the slot.
func (c *SlotClock) Tick() uint64 {
	c.intervals++
	return c.intervals % types.IntervalsPerSlot
}

// TargetIntervals computes how many intervals should have elapsed by the given unix time.
func (c *SlotClock) TargetIntervals(unixTime uint64) uint64 {
	if unixTime < c.genesisTime {
		return 0
	}
	return (unixTime - c.genesisTime) / types.SecondsPerInterval
}

// SlotStartTime returns the unix timestamp at which the given slot begins.
func (c *SlotClock) SlotStartTime(slot types.Slot) uint64 {
	return c.genesisTime + uint64(slot)*types.SecondsPerSlot
}
