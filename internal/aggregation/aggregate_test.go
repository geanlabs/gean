package aggregation

import (
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func TestAggregationMessageRejectsNilData(t *testing.T) {
	if _, _, err := aggregationMessage(nil); err == nil {
		t.Fatal("expected nil attestation data error")
	}
}

func TestAggregationMessageRejectsSlotOverflow(t *testing.T) {
	data := &types.AttestationData{
		Slot:   uint64(^uint32(0)) + 1,
		Head:   &types.Checkpoint{},
		Target: &types.Checkpoint{},
		Source: &types.Checkpoint{},
	}

	if _, _, err := aggregationMessage(data); err == nil {
		t.Fatal("expected slot overflow error")
	}
}

func TestAggregationMessageBuildsRootAndSlot(t *testing.T) {
	data := &types.AttestationData{
		Slot:   12,
		Head:   &types.Checkpoint{Slot: 12},
		Target: &types.Checkpoint{Slot: 10},
		Source: &types.Checkpoint{Slot: 8},
	}
	wantRoot, err := data.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash data: %v", err)
	}

	root, slot, err := aggregationMessage(data)
	if err != nil {
		t.Fatalf("aggregation message: %v", err)
	}
	if root != wantRoot || slot != 12 {
		t.Fatalf("root=%x slot=%d, want %x/12", root, slot, wantRoot)
	}
}
