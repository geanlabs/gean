package node

import (
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/p2p"
	"github.com/geanlabs/gean/internal/types"
)

func TestOnGossipAggregatedAttestationRejectsMissingProof(t *testing.T) {
	e := makeTestEngine()
	head := e.Store.Head()
	attData := &types.AttestationData{
		Slot:   0,
		Source: &types.Checkpoint{Root: head, Slot: 0},
		Target: &types.Checkpoint{Root: head, Slot: 0},
		Head:   &types.Checkpoint{Root: head, Slot: 0},
	}

	e.onGossipAggregatedAttestation(&types.SignedAggregatedAttestation{
		Data:  attData,
		Proof: nil,
	})
	if e.Store.NewPayloads.Len() != 0 {
		t.Fatalf("payloads=%d, want 0 after nil proof", e.Store.NewPayloads.Len())
	}

	e.onGossipAggregatedAttestation(&types.SignedAggregatedAttestation{
		Data: attData,
		Proof: &types.SingleMessageAggregate{
			Participants: types.NewBitlistSSZ(0),
			Proof:        nil,
		},
	})
	if e.Store.NewPayloads.Len() != 0 {
		t.Fatalf("payloads=%d, want 0 after empty proof", e.Store.NewPayloads.Len())
	}
}

func TestValidateGossipBlock(t *testing.T) {
	e := makeTestEngine()
	// Wall clock at slot 5. The store clock is left at genesis, as when the loop
	// lags: it must not decide the horizon.
	genesis := uint64(time.Now().Unix()) - 5*types.MillisecondsPerSlot/1000
	e.Store.SetConfig(&types.ChainConfig{GenesisTime: genesis})
	e.Store.SetLatestFinalized(&types.Checkpoint{Root: e.Store.Head(), Slot: 3})

	for _, tc := range []struct {
		slot uint64
		want p2p.Verdict
	}{
		{3, p2p.Ignore}, // at finalized
		{4, p2p.Accept},
		{6, p2p.Accept}, // one slot ahead, inside the import horizon
		{7, p2p.Ignore}, // beyond it
	} {
		block := &types.SignedBlock{Block: &types.Block{Slot: tc.slot, Body: &types.BlockBody{}}}
		if got := e.ValidateGossipBlock(block); got != tc.want {
			t.Errorf("slot %d: verdict=%d, want %d", tc.slot, got, tc.want)
		}
	}
}

func TestValidateGossipAttestationVerdicts(t *testing.T) {
	e := makeTestEngine()
	genesis := e.Store.Head()
	cp := func(root [32]byte, slot uint64) *types.Checkpoint { return &types.Checkpoint{Root: root, Slot: slot} }
	var unknown [32]byte
	unknown[0] = 0xEE

	for _, tc := range []struct {
		name string
		data *types.AttestationData
		want p2p.Verdict
	}{
		{"valid", &types.AttestationData{Head: cp(genesis, 0), Target: cp(genesis, 0), Source: cp(genesis, 0)}, p2p.Accept},
		// Still delivered, so it reaches the pending buffer and triggers a fetch.
		{"unknown head", &types.AttestationData{Slot: 1, Head: cp(unknown, 1), Target: cp(genesis, 0), Source: cp(genesis, 0)}, p2p.Accept},
		{"source after target", &types.AttestationData{Slot: 2, Head: cp(genesis, 2), Target: cp(genesis, 0), Source: cp(genesis, 1)}, p2p.Reject},
		// Contradicting slots are rejected before any block is looked up.
		{"source after target, unknown roots", &types.AttestationData{Slot: 2, Head: cp(unknown, 2), Target: cp(unknown, 0), Source: cp(unknown, 1)}, p2p.Reject},
		{"head older than target, unknown roots", &types.AttestationData{Slot: 3, Head: cp(unknown, 1), Target: cp(unknown, 2), Source: cp(unknown, 0)}, p2p.Reject},
		{"slot before head, unknown roots", &types.AttestationData{Slot: 1, Head: cp(unknown, 2), Target: cp(unknown, 2), Source: cp(unknown, 0)}, p2p.Reject},
		{"head slot disagrees with its block", &types.AttestationData{Slot: 3, Head: cp(genesis, 3), Target: cp(genesis, 0), Source: cp(genesis, 0)}, p2p.Reject},
		// Judged by the lagging store clock, so it is left to the engine.
		{"ahead of the store clock", &types.AttestationData{Slot: 10, Head: cp(genesis, 0), Target: cp(genesis, 0), Source: cp(genesis, 0)}, p2p.Accept},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := e.ValidateGossipAttestation(&types.SignedAttestation{Data: tc.data}); got != tc.want {
				t.Fatalf("verdict=%d, want %d", got, tc.want)
			}
		})
	}

	// A vote for a slot-4 block on a branch beside the slot-2 finalized block is
	// dead to this node, but not invalid: ignored, not rejected.
	var finalizedRoot, fork [32]byte
	finalizedRoot[0], fork[0] = 0xF2, 0xF4
	e.Store.InsertBlockHeader(finalizedRoot, &types.BlockHeader{Slot: 2, ParentRoot: genesis})
	e.Store.InsertBlockHeader(fork, &types.BlockHeader{Slot: 4, ParentRoot: genesis})
	e.Store.SetLatestFinalized(&types.Checkpoint{Root: finalizedRoot, Slot: 2})
	stale := &types.AttestationData{Slot: 4, Head: cp(fork, 4), Target: cp(fork, 4), Source: cp(genesis, 0)}
	if got := e.ValidateGossipAttestation(&types.SignedAttestation{Data: stale}); got != p2p.Ignore {
		t.Fatalf("vote off the finalized chain: verdict=%d, want ignore", got)
	}
}

func TestValidateGossipAggregatedAttestationVerdicts(t *testing.T) {
	e := makeTestEngine()
	genesis := e.Store.Head()
	valid := &types.AttestationData{
		Head: &types.Checkpoint{Root: genesis}, Target: &types.Checkpoint{Root: genesis}, Source: &types.Checkpoint{Root: genesis},
	}
	var unknown [32]byte
	unknown[0] = 0xEE
	contradicting := &types.AttestationData{
		Slot: 2, Head: &types.Checkpoint{Root: unknown, Slot: 2},
		Target: &types.Checkpoint{Root: unknown}, Source: &types.Checkpoint{Root: unknown, Slot: 1},
	}
	participants := types.NewBitlistSSZ(1)
	types.BitlistSet(participants, 0)
	proof := &types.SingleMessageAggregate{Participants: participants, Proof: []byte{1}}

	for _, tc := range []struct {
		name  string
		data  *types.AttestationData
		proof *types.SingleMessageAggregate
		want  p2p.Verdict
	}{
		{"no participants", valid, &types.SingleMessageAggregate{Participants: types.NewBitlistSSZ(1), Proof: []byte{1}}, p2p.Reject},
		{"no proof bytes", valid, &types.SingleMessageAggregate{Participants: participants}, p2p.Reject},
		{"contradicting slots, unknown roots", contradicting, proof, p2p.Reject},
		{"non-empty", valid, proof, p2p.Accept},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agg := &types.SignedAggregatedAttestation{Data: tc.data, Proof: tc.proof}
			if got := e.ValidateGossipAggregatedAttestation(agg); got != tc.want {
				t.Fatalf("verdict=%d, want %d", got, tc.want)
			}
		})
	}
}
