package aggregation

import (
	"errors"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/attestationproof"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

func TestProofMergerRejectsUnmergeableInput(t *testing.T) {
	proof := func(ids ...uint64) *types.SingleMessageAggregate {
		return &types.SingleMessageAggregate{Participants: types.BitlistFromIndices(ids), Proof: []byte{0x01}}
	}
	validators := func(n int) *types.State {
		state := &types.State{Validators: make([]*types.Validator, n)}
		for i := range state.Validators {
			state.Validators[i] = &types.Validator{}
		}
		return state
	}
	cache := xmss.NewPubKeyCache()
	defer cache.Close()

	tests := []struct {
		name    string
		cache   *xmss.PubKeyCache
		proofs  []*types.SingleMessageAggregate
		slot    uint64
		state   *types.State
		wantMsg string
	}{
		{name: "nil cache", slot: 1, proofs: []*types.SingleMessageAggregate{proof(0), proof(1)}, state: validators(2), wantMsg: "pubkey cache is nil"},
		{name: "slot overflow", proofs: []*types.SingleMessageAggregate{proof(0), proof(1)}, slot: uint64(^uint32(0)) + 1, state: &types.State{}, wantMsg: "overflows uint32"},
		{name: "participant out of range", slot: 1, cache: cache, proofs: []*types.SingleMessageAggregate{proof(0), proof(1)}, state: validators(1), wantMsg: "exceeds validator count"},
		{name: "malformed proof", slot: 1, cache: cache, proofs: []*types.SingleMessageAggregate{proof(0), {Participants: types.BitlistFromIndices([]uint64{1})}}, state: validators(2), wantMsg: "malformed child proof"},
		{name: "overlapping participants", slot: 1, cache: cache, proofs: []*types.SingleMessageAggregate{proof(0, 1), proof(1, 2)}, state: validators(3), wantMsg: "appears in multiple proofs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &types.AttestationData{Slot: tt.slot, Head: &types.Checkpoint{}, Target: &types.Checkpoint{}, Source: &types.Checkpoint{}}
			merged, err := NewProofMerger(tt.cache).Merge(tt.proofs, data, tt.state)
			if merged != nil {
				t.Fatalf("proof=%v, want nil", merged)
			}
			if !errors.Is(err, attestationproof.ErrMergeUnavailable) || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("error=%v, want ErrMergeUnavailable containing %q", err, tt.wantMsg)
			}
		})
	}
}
