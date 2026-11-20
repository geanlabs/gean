package clock

import (
	"sync"
	"time"
)

// Fake is a Clock for tests. Time stands still until Advance or Set moves it;
// timers fire when time reaches their deadline. It is safe for concurrent use.
type Fake struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

// NewFake returns a Fake clock reading start.
func NewFake(start time.Time) *Fake {
	return &Fake{now: start}
}

// Now returns the fake current time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// NewTimer returns a timer that fires once the fake time reaches Now()+d.
// A d of zero or less fires immediately.
func (f *Fake) NewTimer(d time.Duration) Timer {
	f.mu.Lock()
	defer f.mu.Unlock()
	t := &fakeTimer{clock: f, ch: make(chan time.Time, 1)}
	t.armLocked(d)
	f.timers = append(f.timers, t)
	return t
}

// Advance moves the fake time forward by d and fires every timer that is due.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
	f.fireDueLocked()
}

// Set moves the fake time to t (forwards or backwards) and fires every timer
// that is due.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
	f.fireDueLocked()
}

// fireDueLocked fires active timers whose deadline is at or before now. Like
// time.Timer, a fire is a non-blocking send into a one-slot channel.
func (f *Fake) fireDueLocked() {
	for _, t := range f.timers {
		if t.active && !t.deadline.After(f.now) {
			t.active = false
			select {
			case t.ch <- f.now:
			default:
			}
		}
	}
}

type fakeTimer struct {
	clock    *Fake
	ch       chan time.Time
	deadline time.Time
	active   bool
}

func (t *fakeTimer) C() <-chan time.Time { return t.ch }

func (t *fakeTimer) Reset(d time.Duration) bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	wasActive := t.active
	t.armLocked(d)
	return wasActive
}

func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	wasActive := t.active
	t.active = false
	return wasActive
}

// armLocked sets the deadline d after the current fake time and fires at once
// if it is already due. The caller holds the clock's lock.
func (t *fakeTimer) armLocked(d time.Duration) {
	t.deadline = t.clock.now.Add(d)
	t.active = true
	if !t.deadline.After(t.clock.now) {
		t.active = false
		select {
		case t.ch <- t.clock.now:
		default:
		}
	}
}
