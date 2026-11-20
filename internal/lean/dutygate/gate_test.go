package dutygate

import "testing"

func TestDutyGateStartsOpen(t *testing.T) {
	g := New()
	if g.IsClosed() {
		t.Fatal("new gate should start open")
	}
}

func TestDutyGateAllowsWhenInsideThreshold(t *testing.T) {
	g := New()
	if !g.Decide("attestation", 10, 6, 10) {
		t.Fatal("lag at threshold should be allowed")
	}
	if g.IsClosed() {
		t.Fatal("gate should remain open at threshold")
	}
}

func TestDutyGateClosesWhenLagCrossesThreshold(t *testing.T) {
	g := New()
	if g.Decide("attestation", 10, 5, 5) {
		t.Fatal("lag past threshold should be denied")
	}
	if !g.IsClosed() {
		t.Fatal("gate should close after crossing threshold")
	}
}

func TestDutyGateHysteresisStaysClosedAboveReopenThreshold(t *testing.T) {
	g := New()
	g.Decide("attestation", 10, 5, 5)
	if !g.IsClosed() {
		t.Fatal("setup: gate should be closed")
	}

	if g.Decide("attestation", 10, 7, 7) {
		t.Fatal("lag above reopen threshold should keep the gate closed")
	}
	if !g.IsClosed() {
		t.Fatal("gate should still be closed")
	}
}

func TestDutyGateHysteresisReopensAtThreshold(t *testing.T) {
	g := New()
	g.Decide("attestation", 10, 5, 5)
	if !g.IsClosed() {
		t.Fatal("setup: gate should be closed")
	}

	if !g.Decide("attestation", 10, 8, 8) {
		t.Fatal("lag at reopen threshold should reopen the gate")
	}
	if g.IsClosed() {
		t.Fatal("gate should have reopened")
	}
}

func TestDutyGateNetworkStallOverridesGate(t *testing.T) {
	g := New()
	g.Decide("attestation", 10, 5, 5)
	if !g.IsClosed() {
		t.Fatal("setup: gate should be closed")
	}

	if !g.Decide("attestation", 20, 5, 5) {
		t.Fatal("network stall must override the closed gate")
	}
	if g.IsClosed() {
		t.Fatal("network stall should reopen the gate")
	}
}

func TestDutyGateMaxStoredBehindHeadDoesNotTriggerNetworkStall(t *testing.T) {
	var events []Event
	g := New(func(event Event) {
		events = append(events, event)
	})
	g.Decide("attestation", 10, 5, 5)
	if !g.IsClosed() {
		t.Fatal("setup: gate should be closed")
	}

	if g.Decide("attestation", 10, 5, 1) {
		t.Fatal("max stored slot behind head should not look like a network stall")
	}
	if !g.IsClosed() {
		t.Fatal("gate should remain closed on local lag")
	}
	if len(events) != 1 {
		t.Fatalf("events=%v, want only the initial local lag event", events)
	}
}

func TestDutyGateNetworkStallKeepsOpenGateOpen(t *testing.T) {
	g := New()
	if !g.Decide("block", 10, 1, 1) {
		t.Fatal("network stall should keep duties live")
	}
	if g.IsClosed() {
		t.Fatal("network stall should not close the gate")
	}
}

func TestDutyGateHeadAheadOfWallSaturatesAtZero(t *testing.T) {
	g := New()
	if !g.Decide("attestation", 10, 100, 100) {
		t.Fatal("head ahead of wall should allow duties")
	}
}

func TestDutyGateTransitionEventsOnlyOnEdges(t *testing.T) {
	var events []Event
	g := New(func(event Event) {
		events = append(events, event)
	})

	for i := range 5 {
		if !g.Decide("attestation", 10, 9, 9) {
			t.Fatalf("iter %d: should remain open", i)
		}
		if g.IsClosed() {
			t.Fatalf("iter %d: should not toggle to closed", i)
		}
	}
	if len(events) != 0 {
		t.Fatalf("events=%v, want none", events)
	}

	g.Decide("attestation", 10, 5, 5)
	if len(events) != 1 || events[0].Reason != ReasonLocalLag {
		t.Fatalf("events=%v, want one local lag event", events)
	}

	for i := range 5 {
		if g.Decide("attestation", 10, 7, 7) {
			t.Fatalf("iter %d: should remain closed", i)
		}
		if !g.IsClosed() {
			t.Fatalf("iter %d: should not toggle to open", i)
		}
	}
	if len(events) != 1 {
		t.Fatalf("events=%v, want no repeated closed events", events)
	}

	if !g.Decide("attestation", 10, 8, 8) {
		t.Fatal("lag at reopen threshold should reopen the gate")
	}
	if len(events) != 2 || events[1].Reason != ReasonCaughtUp {
		t.Fatalf("events=%v, want caught up event", events)
	}
}

func TestDutyGateNetworkStallEmitsTransitionEvent(t *testing.T) {
	var events []Event
	g := New(func(event Event) {
		events = append(events, event)
	})

	g.Decide("attestation", 10, 5, 5)
	if !g.Decide("attestation", 20, 5, 5) {
		t.Fatal("network stall should reopen the gate")
	}

	if len(events) != 2 {
		t.Fatalf("events=%v, want close and network stall reopen", events)
	}
	if events[1].Reason != ReasonNetworkStall {
		t.Fatalf("second event reason=%q, want %q", events[1].Reason, ReasonNetworkStall)
	}
	if events[1].MaxStoredSlot != 5 || events[1].NetworkLag != 15 {
		t.Fatalf("network stall event=%+v has wrong slot data", events[1])
	}
}

func TestSlotLagSaturates(t *testing.T) {
	if got := slotLag(10, 100); got != 0 {
		t.Fatalf("slotLag=%d, want 0", got)
	}
}
