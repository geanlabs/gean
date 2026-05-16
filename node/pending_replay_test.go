package node

import (
	"testing"
)

// TestReplayPendingAttestations_DrainsBucket verifies the replay helper
// empties the bucket for the given head root. We can't easily verify that
// the spawned goroutines successfully re-validate the attestations without
// a full Engine + Store + XMSS, so this test focuses on the drain semantics:
// after replayPendingAttestations returns, the buffer must no longer hold
// the entries for that root.
func TestReplayPendingAttestations_DrainsBucket(t *testing.T) {
	e := &Engine{
		PendingAttestations: NewPendingAttestationBuffer(8, 64),
	}

	var head [32]byte
	head[0] = 0x42

	// Buffer 3 attestations under one head root, plus one under a different
	// root to confirm the drain is targeted (does not nuke unrelated entries).
	for _, slot := range []uint64{10, 11, 12} {
		e.PendingAttestations.Add(head, makeAttForHead(slot, head))
	}
	var otherHead [32]byte
	otherHead[0] = 0x99
	e.PendingAttestations.Add(otherHead, makeAttForHead(20, otherHead))

	if e.PendingAttestations.Total() != 4 {
		t.Fatalf("setup: total=%d, want 4", e.PendingAttestations.Total())
	}

	// Replay for `head` should drain only that bucket. Note that the spawned
	// onGossipAttestation goroutines will fail/return quickly because the
	// Engine has no Store/AggCtl; we don't depend on their behavior here,
	// only on the synchronous Drain that happens inside the helper.
	//
	// To avoid the goroutines panicking on nil AggCtl, we wire a controller
	// disabled by default — onGossipAttestation early-returns on AggCtl.Get()
	// being false, which is the safe path for this unit-level test.
	e.AggCtl = NewAggregatorController(false)
	e.replayPendingAttestations(head)

	if e.PendingAttestations.Total() != 1 {
		t.Fatalf("after replay: total=%d, want 1 (only the otherHead entry should remain)",
			e.PendingAttestations.Total())
	}
	if e.PendingAttestations.Len() != 1 {
		t.Fatalf("after replay: len=%d, want 1 bucket left", e.PendingAttestations.Len())
	}
}

// TestReplayPendingAttestations_NoBucketIsNoOp verifies the helper handles
// an unknown head root cleanly — no error, no panic, no spurious work.
func TestReplayPendingAttestations_NoBucketIsNoOp(t *testing.T) {
	e := &Engine{
		PendingAttestations: NewPendingAttestationBuffer(8, 64),
		AggCtl:              NewAggregatorController(false),
	}

	var head [32]byte
	head[0] = 0xff

	// Nothing buffered — must not panic.
	e.replayPendingAttestations(head)

	if e.PendingAttestations.Total() != 0 {
		t.Fatalf("total=%d, want 0", e.PendingAttestations.Total())
	}
}
