package sim

import "time"

// Clock is a manual clock: time moves only when the simulation advances it.
// It implements node.Clock.
type Clock struct {
	now time.Time
}

func (c *Clock) Now() time.Time { return c.now }

func (c *Clock) advance(d time.Duration) { c.now = c.now.Add(d) }
