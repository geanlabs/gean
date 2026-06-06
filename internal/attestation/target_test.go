package attestation_test

import (
	"testing"

	"github.com/geanlabs/gean/internal/attestation"
	"github.com/geanlabs/gean/internal/types"
)

func TestGetAttestationTargetEmptyWhenHeadMissing(t *testing.T) {
	s := makeValidationStore()
	s.SetHead([32]byte{0x9}) // no header inserted for this root

	cp := attestation.GetAttestationTarget(s)
	if cp.Root != ([32]byte{}) || cp.Slot != 0 {
		t.Fatalf("expected empty checkpoint, got %+v", cp)
	}
}

// The first walk-back loop rewinds the head toward the safe-target slot (up to
// JustificationLookbackSlots steps). With a 3-block chain head(10)->9->8 and the
// safe target at slot 8, the target should resolve to the slot-8 ancestor.
func TestGetAttestationTargetWalksBackToSafeTarget(t *testing.T) {
	s := makeValidationStore()
	h10 := [32]byte{10}
	h9 := [32]byte{9}
	h8 := [32]byte{8}
	s.InsertBlockHeader(h10, &types.BlockHeader{Slot: 10, ParentRoot: h9})
	s.InsertBlockHeader(h9, &types.BlockHeader{Slot: 9, ParentRoot: h8})
	s.InsertBlockHeader(h8, &types.BlockHeader{Slot: 8})

	s.SetHead(h10)
	s.SetSafeTarget(h8)
	s.SetLatestFinalized(&types.Checkpoint{Slot: 8}) // justifiability loop is a no-op at slot 8
	s.SetLatestJustified(&types.Checkpoint{Slot: 0})

	cp := attestation.GetAttestationTarget(s)
	if cp.Slot != 8 || cp.Root != h8 {
		t.Fatalf("target = %+v, want slot 8 root %x", cp, h8)
	}
}
