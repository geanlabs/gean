package node

import (
	"testing"

	"github.com/geanlabs/gean/pending"
	"github.com/geanlabs/gean/types"
)

func TestReplayPendingAttestations_DrainsBucket(t *testing.T) {
	e := &Engine{
		pendingAttestations: pending.NewAttestationBuffer(8, 64),
		attestationCh:       make(chan *types.SignedAttestation, 8),
	}

	var head [32]byte
	head[0] = 0x42

	for _, slot := range []uint64{10, 11, 12} {
		e.pendingAttestations.Add(head, makeAttForHead(slot, head))
	}
	var otherHead [32]byte
	otherHead[0] = 0x99
	e.pendingAttestations.Add(otherHead, makeAttForHead(20, otherHead))

	if e.pendingAttestations.Total() != 4 {
		t.Fatalf("setup: total=%d, want 4", e.pendingAttestations.Total())
	}

	e.replayPendingAttestations(head)

	if e.pendingAttestations.Total() != 1 {
		t.Fatalf("after replay: total=%d, want 1 (only the otherHead entry should remain)",
			e.pendingAttestations.Total())
	}
	if e.pendingAttestations.Len() != 1 {
		t.Fatalf("after replay: len=%d, want 1 bucket left", e.pendingAttestations.Len())
	}
	if got := len(e.attestationCh); got != 3 {
		t.Fatalf("replayed=%d, want 3 re-queued for verification", got)
	}
}

func TestReplayPendingAttestations_NoBucketIsNoOp(t *testing.T) {
	e := &Engine{
		pendingAttestations: pending.NewAttestationBuffer(8, 64),
	}

	var head [32]byte
	head[0] = 0xff

	e.replayPendingAttestations(head)

	if e.pendingAttestations.Total() != 0 {
		t.Fatalf("total=%d, want 0", e.pendingAttestations.Total())
	}
}
