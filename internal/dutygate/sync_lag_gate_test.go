package dutygate

import "testing"

func TestDutyGate_StartsOpen(t *testing.T) {
	g := New()
	if g.IsClosed() {
		t.Fatal("new gate should start open")
	}
}

func TestDutyGate_AllowsWhenInsideThreshold(t *testing.T) {
	g := New()
	// lag = 4 = SyncLagThreshold — at the boundary, still allowed.
	if !g.Decide("attestation", 10, 6, 10) {
		t.Fatal("lag at threshold should be allowed")
	}
	if g.IsClosed() {
		t.Fatal("gate should remain open at threshold")
	}
}

func TestDutyGate_ClosesWhenLagCrossesThreshold(t *testing.T) {
	g := New()
	// lag = 5 > SyncLagThreshold (4) — should close.
	// networkLag = 5 too; not > NetworkStallThreshold (8), so no override.
	if g.Decide("attestation", 10, 5, 5) {
		t.Fatal("lag past threshold should be denied")
	}
	if !g.IsClosed() {
		t.Fatal("gate should close after crossing threshold")
	}
}

func TestDutyGate_Hysteresis_DoesNotReopenAtThreshold(t *testing.T) {
	g := New()
	// Step 1: close the gate.
	g.Decide("attestation", 10, 5, 5) // lag = 5
	if !g.IsClosed() {
		t.Fatal("setup: gate should be closed")
	}

	// Step 2: drop to lag = 3. SyncLagThreshold - HysteresisBand = 2.
	// 3 > 2, so the gate stays closed despite being below the open-threshold.
	if g.Decide("attestation", 10, 7, 7) {
		t.Fatal("hysteresis: lag = 3 should keep the gate closed")
	}
	if !g.IsClosed() {
		t.Fatal("gate should still be closed")
	}
}

func TestDutyGate_Hysteresis_ReopensBelowBand(t *testing.T) {
	g := New()
	g.Decide("attestation", 10, 5, 5) // close it
	if !g.IsClosed() {
		t.Fatal("setup: gate should be closed")
	}

	// lag = 2 = SyncLagThreshold - HysteresisBand. Should reopen.
	if !g.Decide("attestation", 10, 8, 8) {
		t.Fatal("hysteresis: lag = 2 should reopen the gate")
	}
	if g.IsClosed() {
		t.Fatal("gate should have reopened")
	}
}

func TestDutyGate_NetworkStallOverridesGate(t *testing.T) {
	g := New()
	// Close the gate first.
	g.Decide("attestation", 10, 5, 5)
	if !g.IsClosed() {
		t.Fatal("setup: gate should be closed")
	}

	// Now: lag = 5 (still > threshold) but network_lag = 9 > NetworkStallThreshold.
	// Duties must keep firing so the chain can advance through the gap.
	if !g.Decide("attestation", 10, 5, 1) {
		t.Fatal("network stall must override the closed gate")
	}
	if g.IsClosed() {
		t.Fatal("network stall should reopen the gate")
	}
}

func TestDutyGate_NetworkStallKeepsOpenGateOpen(t *testing.T) {
	g := New()
	// network_lag = 9 > 8, lag = 9. Gate was open. Stays open via stall branch.
	if !g.Decide("block", 10, 1, 1) {
		t.Fatal("network stall should keep duties live")
	}
	if g.IsClosed() {
		t.Fatal("network stall should not close the gate")
	}
}

func TestDutyGate_HeadAheadOfWallSaturatesAtZero(t *testing.T) {
	g := New()
	// head_slot ahead of wall_slot — lag saturates at 0, never trips threshold.
	if !g.Decide("attestation", 10, 100, 100) {
		t.Fatal("head ahead of wall should allow duties")
	}
}

func TestDutyGate_TransitionLogsOnlyOnEdges(t *testing.T) {
	// This test cannot easily assert log output but at minimum verifies the
	// internal state machine: repeated open->open or closed->closed calls
	// don't toggle .closed.
	g := New()
	for i := range 5 {
		// lag = 1, well inside threshold. Should stay open every iteration.
		if !g.Decide("attestation", 10, 9, 9) {
			t.Fatalf("iter %d: should remain open", i)
		}
		if g.IsClosed() {
			t.Fatalf("iter %d: should not toggle to closed", i)
		}
	}

	// Force close.
	g.Decide("attestation", 10, 5, 5)
	for i := range 5 {
		// lag = 3 — above re-open band, gate stays closed.
		if g.Decide("attestation", 10, 7, 7) {
			t.Fatalf("iter %d: should remain closed", i)
		}
		if !g.IsClosed() {
			t.Fatalf("iter %d: should not toggle to open", i)
		}
	}
}
