package node

import (
	"testing"

	"github.com/geanlabs/gean/internal/pending"
)

func TestReplayPendingAttestations_DrainsBucket(t *testing.T) {
	e := &Engine{
		PendingAttestations: pending.NewAttestationBuffer(8, 64),
	}

	var head [32]byte
	head[0] = 0x42

	for _, slot := range []uint64{10, 11, 12} {
		e.PendingAttestations.Add(head, makeAttForHead(slot, head))
	}
	var otherHead [32]byte
	otherHead[0] = 0x99
	e.PendingAttestations.Add(otherHead, makeAttForHead(20, otherHead))

	if e.PendingAttestations.Total() != 4 {
		t.Fatalf("setup: total=%d, want 4", e.PendingAttestations.Total())
	}

	e.replayPendingAttestations(head)

	if e.PendingAttestations.Total() != 1 {
		t.Fatalf("after replay: total=%d, want 1 (only the otherHead entry should remain)",
			e.PendingAttestations.Total())
	}
	if e.PendingAttestations.Len() != 1 {
		t.Fatalf("after replay: len=%d, want 1 bucket left", e.PendingAttestations.Len())
	}
}

func TestReplayPendingAttestations_NoBucketIsNoOp(t *testing.T) {
	e := &Engine{
		PendingAttestations: pending.NewAttestationBuffer(8, 64),
	}

	var head [32]byte
	head[0] = 0xff

	e.replayPendingAttestations(head)

	if e.PendingAttestations.Total() != 0 {
		t.Fatalf("total=%d, want 0", e.PendingAttestations.Total())
	}
}
