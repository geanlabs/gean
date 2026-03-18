package node

import (
	"time"

	"github.com/geanlabs/gean/types"
)

// Clock tracks slot and interval timing relative to genesis.
type Clock struct {
	GenesisTime uint64
}

// NewClock creates a clock from genesis time (unix seconds).
func NewClock(genesisTime uint64) *Clock {
	return &Clock{GenesisTime: genesisTime}
}

func (c *Clock) genesisTimeMillis() uint64 {
	return c.GenesisTime * 1000
}

func (c *Clock) nowMillis() uint64 {
	return uint64(time.Now().UnixMilli())
}

// IsBeforeGenesis returns true if the current time is before genesis.
func (c *Clock) IsBeforeGenesis() bool {
	return c.nowMillis() < c.genesisTimeMillis()
}

// CurrentSlot returns the current slot number, or 0 if before genesis.
func (c *Clock) CurrentSlot() uint64 {
	now := c.nowMillis()
	genesisTimeMillis := c.genesisTimeMillis()
	if now < genesisTimeMillis {
		return 0
	}
	elapsed := now - genesisTimeMillis
	return elapsed / types.MillisecondsPerSlot
}

// CurrentInterval returns the current interval within the slot (0-4), or 0 if before genesis.
func (c *Clock) CurrentInterval() uint64 {
	now := c.nowMillis()
	genesisTimeMillis := c.genesisTimeMillis()
	if now < genesisTimeMillis {
		return 0
	}
	elapsed := now - genesisTimeMillis
	return (elapsed % types.MillisecondsPerSlot) / types.MillisecondsPerInterval
}

// CurrentTime returns the current unix time in milliseconds.
func (c *Clock) CurrentTime() uint64 {
	return c.nowMillis()
}

// DurationUntilNextInterval returns the time until the next genesis-aligned interval boundary.
func (c *Clock) DurationUntilNextInterval() time.Duration {
	now := c.nowMillis()
	genesisTimeMillis := c.genesisTimeMillis()
	if now < genesisTimeMillis {
		return time.Duration(genesisTimeMillis-now) * time.Millisecond
	}

	elapsed := now - genesisTimeMillis
	timeIntoInterval := elapsed % types.MillisecondsPerInterval
	if timeIntoInterval == 0 {
		return 0
	}

	return time.Duration(types.MillisecondsPerInterval-timeIntoInterval) * time.Millisecond
}

// SlotTicker returns a channel that fires at the start of each interval.
func (c *Clock) SlotTicker() *time.Ticker {
	return time.NewTicker(time.Duration(types.MillisecondsPerInterval) * time.Millisecond)
}
