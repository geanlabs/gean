// Package clock provides time-to-slot conversion for Lean Consensus.
//
// The slot clock bridges wall-clock time to the discrete slot-based time
// model used by consensus. Every node must agree on slot boundaries to
// coordinate block proposals and attestations.
package clock

import (
	"time"

	"github.com/devylongs/gean/types"
)

// Interval is the count of intervals since genesis (matches Store.time).
type Interval uint64

// SlotClock converts wall-clock time to consensus slots and intervals.
// All time values are in seconds (Unix timestamps).
type SlotClock struct {
	GenesisTime uint64           // Unix timestamp when slot 0 began
	timeFunc    func() time.Time // Injectable for testing
}

// New creates a SlotClock with the given genesis time.
func New(genesisTime uint64) *SlotClock {
	return &SlotClock{
		GenesisTime: genesisTime,
		timeFunc:    time.Now,
	}
}

// NewWithTimeFunc creates a SlotClock with a custom time source (for testing).
func NewWithTimeFunc(genesisTime uint64, timeFunc func() time.Time) *SlotClock {
	return &SlotClock{
		GenesisTime: genesisTime,
		timeFunc:    timeFunc,
	}
}

// secondsSinceGenesis returns seconds elapsed since genesis (0 if before genesis).
func (c *SlotClock) secondsSinceGenesis() uint64 {
	now := uint64(c.timeFunc().Unix())
	if now < c.GenesisTime {
		return 0
	}
	return now - c.GenesisTime
}

// CurrentSlot returns the current slot number (0 if before genesis).
func (c *SlotClock) CurrentSlot() types.Slot {
	return types.Slot(c.secondsSinceGenesis() / types.SecondsPerSlot)
}

// CurrentInterval returns the current interval within the slot (0-3).
func (c *SlotClock) CurrentInterval() Interval {
	secondsIntoSlot := c.secondsSinceGenesis() % types.SecondsPerSlot
	return Interval(secondsIntoSlot / types.SecondsPerInterval)
}

// TotalIntervals returns total intervals elapsed since genesis.
// This is the value expected by Store.time.
func (c *SlotClock) TotalIntervals() Interval {
	return Interval(c.secondsSinceGenesis() / types.SecondsPerInterval)
}

// SlotStartTime returns the Unix timestamp when a given slot starts.
func (c *SlotClock) SlotStartTime(slot types.Slot) uint64 {
	return c.GenesisTime + uint64(slot)*types.SecondsPerSlot
}

// IsBeforeGenesis returns true if current time is before genesis.
func (c *SlotClock) IsBeforeGenesis() bool {
	return uint64(c.timeFunc().Unix()) < c.GenesisTime
}
