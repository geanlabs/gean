// Package clock is the source of wall-clock time for decisions that depend on
// it: which slot it is, when the next interval starts, whether a deadline has
// passed. Code that makes such decisions takes a Clock instead of calling
// time.Now, so tests can drive it with a Fake and move time forward at will.
//
// Measuring how long something took (for metrics and logs) does not need a
// Clock; time.Now and time.Since are fine there.
package clock

import (
	"math"
	"time"
)

// Clock reads the current time and creates timers on the same timeline.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// NewTimer returns a Timer that fires once, after d.
	NewTimer(d time.Duration) Timer
}

// Timer is a single-shot timer, the Clock counterpart of *time.Timer.
type Timer interface {
	// C delivers the time the timer fired.
	C() <-chan time.Time
	// Reset changes the timer to fire after d. It reports whether the timer
	// was active.
	Reset(d time.Duration) bool
	// Stop prevents the timer from firing. It reports whether the timer was
	// active.
	Stop() bool
}

// Real is the system clock.
type Real struct{}

// Now returns time.Now().
func (Real) Now() time.Time { return time.Now() }

// NewTimer returns a timer backed by time.NewTimer.
func (Real) NewTimer(d time.Duration) Timer { return realTimer{time.NewTimer(d)} }

type realTimer struct{ t *time.Timer }

func (r realTimer) C() <-chan time.Time        { return r.t.C }
func (r realTimer) Reset(d time.Duration) bool { return r.t.Reset(d) }
func (r realTimer) Stop() bool                 { return r.t.Stop() }

// UnixMilli is c.Now() in Unix milliseconds, the unit gean's slot arithmetic
// uses.
func UnixMilli(c Clock) uint64 {
	return uint64(c.Now().UnixMilli())
}

// FromUnixMilli is the inverse of UnixMilli.
// Any time gean handles is far below math.MaxInt64 milliseconds.
func FromUnixMilli(ms uint64) time.Time {
	return time.UnixMilli(int64(min(ms, math.MaxInt64)))
}
