package importer

import (
	"testing"

	"github.com/geanlabs/gean/internal/core/pending"
	"github.com/geanlabs/gean/internal/lean/types"
)

func TestReplayPendingAttestations_DrainsBucket(t *testing.T) {
	imp := &Importer{
		pendingAttestations: pending.NewAttestationBuffer(8, 64, types.AttestationSlot),
	}

	var head [32]byte
	head[0] = 0x42

	for _, slot := range []uint64{10, 11, 12} {
		imp.pendingAttestations.Add(head, makeAttForHead(slot, head))
	}
	var otherHead [32]byte
	otherHead[0] = 0x99
	imp.pendingAttestations.Add(otherHead, makeAttForHead(20, otherHead))

	if imp.pendingAttestations.Total() != 4 {
		t.Fatalf("setup: total=%d, want 4", imp.pendingAttestations.Total())
	}

	imp.replayPendingAttestations(head)

	if imp.pendingAttestations.Total() != 1 {
		t.Fatalf("after replay: total=%d, want 1 (only the otherHead entry should remain)",
			imp.pendingAttestations.Total())
	}
	if imp.pendingAttestations.Len() != 1 {
		t.Fatalf("after replay: len=%d, want 1 bucket left", imp.pendingAttestations.Len())
	}
}

func TestReplayPendingAttestations_NoBucketIsNoOp(t *testing.T) {
	imp := &Importer{
		pendingAttestations: pending.NewAttestationBuffer(8, 64, types.AttestationSlot),
	}

	var head [32]byte
	head[0] = 0xff

	imp.replayPendingAttestations(head)

	if imp.pendingAttestations.Total() != 0 {
		t.Fatalf("total=%d, want 0", imp.pendingAttestations.Total())
	}
}
