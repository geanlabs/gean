package aggregation

import (
	"testing"

	"github.com/geanlabs/gean/lean/consensus/types"
	"github.com/geanlabs/gean/lean/internal/pools"
)

func TestApplyAggregationMutationsTargetsNewPool(t *testing.T) {
	s := pools.New()
	participants := types.NewBitlistSSZ(1)
	types.BitlistSet(participants, 0)
	payloads := []pools.PayloadKV{{
		DataRoot: [32]byte{0x01},
		Data:     &types.AttestationData{Head: &types.Checkpoint{}, Source: &types.Checkpoint{}, Target: &types.Checkpoint{}},
		Proof:    &types.SingleMessageAggregate{Participants: participants, Proof: []byte{1}},
	}}

	applyAggregationMutations(s, payloads, nil)

	if s.NewPayloads.Len() != 1 {
		t.Fatalf("NewPayloads.Len() = %d, want 1 (aggregates must enter the new pool)", s.NewPayloads.Len())
	}
	if s.KnownPayloads.Len() != 0 {
		t.Fatalf("KnownPayloads.Len() = %d, want 0 (must wait for accept-attestations promotion)", s.KnownPayloads.Len())
	}
}
