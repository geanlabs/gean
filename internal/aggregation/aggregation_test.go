package aggregation_test

import (
	"testing"

	"github.com/geanlabs/gean/internal/aggregation"
	"github.com/geanlabs/gean/internal/types"
)

func TestAggregationBitsFromValidatorIndices(t *testing.T) {
	bits := aggregation.AggregationBitsFromIndices([]uint64{0, 3, 7})
	if !types.BitlistGet(bits, 0) || !types.BitlistGet(bits, 3) || !types.BitlistGet(bits, 7) {
		t.Fatal("expected bits 0, 3, 7 set")
	}
	if types.BitlistGet(bits, 1) || types.BitlistGet(bits, 5) {
		t.Fatal("bits 1, 5 should not be set")
	}
	if types.BitlistLen(bits) != 8 {
		t.Fatalf("expected length 8, got %d", types.BitlistLen(bits))
	}
}
