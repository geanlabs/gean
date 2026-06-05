package attestationproof

import (
	"errors"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

type fakeMergeProvider struct {
	proof *types.AggregatedSignatureProof
	err   error
}

func (m fakeMergeProvider) Merge(
	_ []*types.AggregatedSignatureProof,
	_ *types.AttestationData,
	_ *types.State,
) (*types.AggregatedSignatureProof, error) {
	return m.proof, m.err
}

func TestSelectSingleProof(t *testing.T) {
	att, sig, ok, err := Select(testData(), []*types.AggregatedSignatureProof{testProof(0, 1, 2)}, nil, nil)
	if err != nil {
		t.Fatalf("select proof: %v", err)
	}
	if !ok || att == nil || sig == nil {
		t.Fatalf("expected proof selection, got ok=%v att=%v sig=%v", ok, att, sig)
	}
	for _, vid := range []uint64{0, 1, 2} {
		if !types.BitlistGet(sig.Participants, vid) {
			t.Fatalf("validator %d not selected", vid)
		}
	}
}

func TestSelectFallsBackWhenMergeUnavailable(t *testing.T) {
	proofs := []*types.AggregatedSignatureProof{
		testProof(0, 1, 2),
		testProof(3, 4, 5),
	}

	att, sig, ok, err := Select(testData(), proofs, nil, nil)
	if !ok || att == nil || sig == nil {
		t.Fatalf("expected single-proof fallback, got ok=%v att=%v sig=%v", ok, att, sig)
	}
	if !errors.Is(err, ErrMergeUnavailable) {
		t.Fatalf("expected merge error, got %v", err)
	}
	for _, vid := range []uint64{0, 1, 2} {
		if !types.BitlistGet(sig.Participants, vid) {
			t.Fatalf("validator %d not selected", vid)
		}
	}
}

func TestSelectUsesInjectedMerger(t *testing.T) {
	merged := &types.AggregatedSignatureProof{
		Participants: types.BitlistFromIndices([]uint64{0, 1, 2, 3}),
		ProofData:    []byte{0xaa},
	}

	att, sig, ok, err := Select(
		testData(),
		[]*types.AggregatedSignatureProof{testProof(0, 1), testProof(2, 3)},
		nil,
		fakeMergeProvider{proof: merged},
	)
	if err != nil {
		t.Fatalf("select proof: %v", err)
	}
	if !ok || att == nil || sig == nil {
		t.Fatalf("expected merged proof, got ok=%v att=%v sig=%v", ok, att, sig)
	}
	for _, vid := range []uint64{0, 1, 2, 3} {
		if !types.BitlistGet(sig.Participants, vid) || !types.BitlistGet(att.AggregationBits, vid) {
			t.Fatalf("validator %d missing from merged proof", vid)
		}
	}

	merged.Participants[0] = 0
	merged.ProofData[0] = 0
	if !types.BitlistGet(sig.Participants, 0) {
		t.Fatal("merged signature aliases provider-owned participants")
	}
	if sig.ProofData[0] != 0xaa {
		t.Fatal("merged signature aliases provider-owned proof data")
	}
}

func TestSelectFallsBackWhenMergerReturnsNilProof(t *testing.T) {
	att, sig, ok, err := Select(
		testData(),
		[]*types.AggregatedSignatureProof{testProof(0, 1), testProof(2, 3)},
		nil,
		fakeMergeProvider{},
	)
	if !ok || att == nil || sig == nil {
		t.Fatalf("expected single-proof fallback, got ok=%v att=%v sig=%v", ok, att, sig)
	}
	if !errors.Is(err, ErrMergeUnavailable) {
		t.Fatalf("error=%v, want ErrMergeUnavailable", err)
	}
	if !types.BitlistGet(sig.Participants, 0) || !types.BitlistGet(sig.Participants, 1) {
		t.Fatal("fallback did not keep best child proof")
	}
}

func TestSelectFallsBackWhenMergerReturnsMalformedProof(t *testing.T) {
	malformed := &types.AggregatedSignatureProof{
		Participants: types.BitlistFromIndices([]uint64{0, 1, 2, 3}),
	}

	att, sig, ok, err := Select(
		testData(),
		[]*types.AggregatedSignatureProof{testProof(0, 1), testProof(2, 3)},
		nil,
		fakeMergeProvider{proof: malformed},
	)
	if !ok || att == nil || sig == nil {
		t.Fatalf("expected single-proof fallback, got ok=%v att=%v sig=%v", ok, att, sig)
	}
	if !errors.Is(err, ErrMergeUnavailable) {
		t.Fatalf("error=%v, want ErrMergeUnavailable", err)
	}
	if !types.BitlistGet(sig.Participants, 0) || !types.BitlistGet(sig.Participants, 1) {
		t.Fatal("fallback did not keep best child proof")
	}
}

func TestSelectProofsGreedyCoverage(t *testing.T) {
	a := testProof(0, 1, 2)
	b := testProof(3, 4, 5, 6)

	selected := selectProofs([]*types.AggregatedSignatureProof{a, b})
	if len(selected) != 2 {
		t.Fatalf("selected %d proofs, want 2", len(selected))
	}
	if selected[0] != b || selected[1] != a {
		t.Fatal("expected proof with most new coverage first")
	}
}

func TestSelectProofsSkipsOverlaps(t *testing.T) {
	a := testProof(0, 1, 2)
	b := testProof(2, 3, 4, 5)

	selected := selectProofs([]*types.AggregatedSignatureProof{a, b})
	if len(selected) != 1 || selected[0] != b {
		t.Fatalf("selected %d proofs, want only the widest disjoint proof", len(selected))
	}
}

func TestSelectProofsSkipsSubsets(t *testing.T) {
	a := testProof(0, 1, 2, 3, 4)
	b := testProof(1, 2)

	selected := selectProofs([]*types.AggregatedSignatureProof{a, b})
	if len(selected) != 1 || selected[0] != a {
		t.Fatalf("selected %d proofs, want only the superset", len(selected))
	}
}

func TestSelectProofsTieUsesInputOrder(t *testing.T) {
	a := testProof(0, 1)
	b := testProof(2, 3)

	selected := selectProofs([]*types.AggregatedSignatureProof{a, b})
	if len(selected) != 2 {
		t.Fatalf("selected %d proofs, want 2", len(selected))
	}
	if selected[0] != a || selected[1] != b {
		t.Fatal("expected deterministic input-order tie break")
	}
}

func TestSelectRejectsMalformedProofs(t *testing.T) {
	emptyProofData := testProof(0)
	emptyProofData.ProofData = nil
	noCoverage := &types.AggregatedSignatureProof{
		Participants: types.BitlistFromIndices(nil),
		ProofData:    []byte{0x01},
	}

	att, sig, ok, err := Select(testData(), []*types.AggregatedSignatureProof{emptyProofData, noCoverage}, nil, nil)
	if ok || att != nil || sig != nil {
		t.Fatalf("expected malformed proofs to be skipped, got ok=%v", ok)
	}
	if !errors.Is(err, ErrNoUsableProofs) {
		t.Fatalf("expected no usable proofs error, got %v", err)
	}
}

func TestSelectCopiesCallerOwnedData(t *testing.T) {
	data := testData()
	proof := testProof(0)

	att, sig, ok, err := Select(data, []*types.AggregatedSignatureProof{proof}, nil, nil)
	if err != nil {
		t.Fatalf("select proof: %v", err)
	}
	if !ok || att == nil || sig == nil {
		t.Fatalf("expected proof selection, got ok=%v att=%v sig=%v", ok, att, sig)
	}

	data.Head.Slot = 99
	proof.Participants[0] = 0
	proof.ProofData[0] = 0

	if att.Data.Head.Slot != 0 {
		t.Fatalf("attestation data head slot=%d, want copied value 0", att.Data.Head.Slot)
	}
	if !types.BitlistGet(att.AggregationBits, 0) || !types.BitlistGet(sig.Participants, 0) {
		t.Fatal("participants changed after caller-owned proof mutation")
	}
	if sig.ProofData[0] != 0x01 {
		t.Fatalf("proof data first byte=0x%x, want copied 0x01", sig.ProofData[0])
	}
}
