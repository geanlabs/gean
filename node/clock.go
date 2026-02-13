package node

import "time"

// Clock tracks slot and interval timing relative to genesis.
type Clock struct {
	GenesisTime uint64
}

// NewClock creates a clock from genesis time (unix seconds).
func NewClock(genesisTime uint64) *Clock {
	return &Clock{GenesisTime: genesisTime}
}

// IsBeforeGenesis returns true if the current time is before genesis.
func (c *Clock) IsBeforeGenesis() bool {
	return uint64(time.Now().Unix()) < c.GenesisTime
}

// CurrentSlot returns the current slot number, or 0 if before genesis.
func (c *Clock) CurrentSlot() uint64 {
	now := uint64(time.Now().Unix())
	if now < c.GenesisTime {
		return 0
	}
	elapsed := now - c.GenesisTime
	return elapsed / 4 // SECONDS_PER_SLOT = 4
}

// CurrentInterval returns the current interval within the slot (0-3), or 0 if before genesis.
func (c *Clock) CurrentInterval() uint64 {
	now := uint64(time.Now().Unix())
	if now < c.GenesisTime {
		return 0
	}
	elapsed := now - c.GenesisTime
	return elapsed % 4 // INTERVALS_PER_SLOT = 4
}

// CurrentTime returns the current unix time in seconds.
func (c *Clock) CurrentTime() uint64 {
	return uint64(time.Now().Unix())
}

// SlotTicker returns a channel that fires at the start of each interval.
func (c *Clock) SlotTicker() <-chan time.Time {
	return time.NewTicker(time.Second).C // 1 second per interval
}
