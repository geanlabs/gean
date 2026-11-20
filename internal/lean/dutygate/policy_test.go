package dutygate

import "testing"

func TestDecideOpenGateClosesOnLocalLag(t *testing.T) {
	next := decide(false, "attestation", 10, 5, 5)
	if next.allow {
		t.Fatal("local lag should deny the duty")
	}
	if !next.closed {
		t.Fatal("local lag should close the gate")
	}
	if next.event == nil || next.event.Reason != ReasonLocalLag {
		t.Fatalf("event=%v, want local lag event", next.event)
	}
}

func TestDecideClosedGateReopensWhenCaughtUp(t *testing.T) {
	next := decide(true, "attestation", 10, 8, 8)
	if !next.allow {
		t.Fatal("caught-up gate should allow the duty")
	}
	if next.closed {
		t.Fatal("caught-up gate should reopen")
	}
	if next.event == nil || next.event.Reason != ReasonCaughtUp {
		t.Fatalf("event=%v, want caught up event", next.event)
	}
}

func TestDecideNetworkStallReopensClosedGate(t *testing.T) {
	next := decide(true, "block", 20, 5, 5)
	if !next.allow {
		t.Fatal("network stall should allow the duty")
	}
	if next.closed {
		t.Fatal("network stall should reopen the gate")
	}
	if next.event == nil || next.event.Reason != ReasonNetworkStall {
		t.Fatalf("event=%v, want network stall event", next.event)
	}
}

func TestDecideNormalizesMaxSeenSlotToHead(t *testing.T) {
	next := decide(true, "block", 10, 5, 1)
	if next.allow {
		t.Fatal("max stored slot behind head should not reopen a locally stale gate")
	}
	if !next.closed {
		t.Fatal("gate should remain closed")
	}
	if next.event != nil {
		t.Fatalf("event=%v, want none", next.event)
	}
}
