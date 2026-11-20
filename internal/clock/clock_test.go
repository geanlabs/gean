package clock

import (
	"testing"
	"time"
)

var start = time.UnixMilli(1_700_000_000_000)

func fired(t Timer) bool {
	select {
	case <-t.C():
		return true
	default:
		return false
	}
}

func TestFakeNowStandsStillUntilAdvanced(t *testing.T) {
	f := NewFake(start)
	if !f.Now().Equal(start) {
		t.Fatalf("Now() = %v, want %v", f.Now(), start)
	}
	f.Advance(1500 * time.Millisecond)
	if got, want := f.Now(), start.Add(1500*time.Millisecond); !got.Equal(want) {
		t.Fatalf("after Advance, Now() = %v, want %v", got, want)
	}
	if got := UnixMilli(f); got != uint64(start.UnixMilli())+1500 {
		t.Fatalf("UnixMilli = %d", got)
	}
}

func TestFakeTimerFiresAtDeadline(t *testing.T) {
	f := NewFake(start)
	timer := f.NewTimer(time.Second)

	f.Advance(999 * time.Millisecond)
	if fired(timer) {
		t.Fatal("timer fired before its deadline")
	}
	f.Advance(time.Millisecond)
	select {
	case at := <-timer.C():
		if want := start.Add(time.Second); !at.Equal(want) {
			t.Fatalf("fired at %v, want %v", at, want)
		}
	default:
		t.Fatal("timer did not fire at its deadline")
	}
	f.Advance(time.Hour)
	if fired(timer) {
		t.Fatal("a single-shot timer fired twice")
	}
}

func TestFakeTimerZeroDurationFiresImmediately(t *testing.T) {
	f := NewFake(start)
	if !fired(f.NewTimer(0)) {
		t.Fatal("a zero-duration timer did not fire at once")
	}
}

func TestFakeTimerStopAndReset(t *testing.T) {
	f := NewFake(start)
	timer := f.NewTimer(time.Second)

	if !timer.Stop() {
		t.Fatal("Stop on an active timer reported inactive")
	}
	f.Advance(2 * time.Second)
	if fired(timer) {
		t.Fatal("a stopped timer fired")
	}

	if timer.Reset(time.Second) {
		t.Fatal("Reset on a stopped timer reported active")
	}
	f.Advance(time.Second)
	if !fired(timer) {
		t.Fatal("a reset timer did not fire")
	}
}

func TestFakeSetFiresDueTimers(t *testing.T) {
	f := NewFake(start)
	timer := f.NewTimer(10 * time.Second)
	f.Set(start.Add(time.Minute))
	if !fired(timer) {
		t.Fatal("Set past the deadline did not fire the timer")
	}
}

func TestRealClockMatchesSystemTime(t *testing.T) {
	before := time.Now()
	got := Real{}.Now()
	if got.Before(before) || got.After(time.Now()) {
		t.Fatalf("Real.Now() = %v, outside the surrounding time.Now() calls", got)
	}
}
