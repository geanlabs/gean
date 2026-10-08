package aggregation

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/shadow"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

const valueTestValidators = 6 // 2/3 is 4 votes

func valueTestRoot(slot int) [32]byte {
	var r [32]byte
	r[0] = byte(slot + 1)
	return r
}

// valueTestState is finalized at slot 0 with slots 1-8 on chain and
// unjustified, and validators 0 and 1 already counted for the slot-1 target.
func valueTestState() *types.State {
	hashes := make([][]byte, 9)
	for i := range hashes {
		r := valueTestRoot(i)
		hashes[i] = r[:]
	}
	tally := types.NewBitlistSSZ(valueTestValidators)
	types.BitlistSet(tally, 0)
	types.BitlistSet(tally, 1)
	target := valueTestRoot(1)
	validators := make([]*types.Validator, valueTestValidators)
	for i := range validators {
		validators[i] = &types.Validator{Index: uint64(i)}
	}
	return &types.State{
		Validators:               validators,
		LatestFinalized:          &types.Checkpoint{Slot: 0, Root: valueTestRoot(0)},
		LatestJustified:          &types.Checkpoint{Slot: 0, Root: valueTestRoot(0)},
		HistoricalBlockHashes:    hashes,
		JustifiedSlots:           types.NewBitlistSSZ(8),
		JustificationsRoots:      [][]byte{target[:]},
		JustificationsValidators: tally,
	}
}

func valueTestData(sourceSlot, targetSlot, headSlot int) *types.AttestationData {
	cp := func(slot int) *types.Checkpoint {
		return &types.Checkpoint{Slot: uint64(slot), Root: valueTestRoot(slot)}
	}
	return &types.AttestationData{Slot: uint64(headSlot), Source: cp(sourceSlot), Target: cp(targetSlot), Head: cp(headSlot)}
}

func TestProofValue(t *testing.T) {
	offChainHead := valueTestData(0, 1, 2)
	offChainHead.Head.Root = [32]byte{0xff}

	for _, tc := range []struct {
		name   string
		data   *types.AttestationData
		voters []uint64
		held   map[uint64]bool
		want   string
	}{
		{"source not justified", valueTestData(2, 3, 3), []uint64{2, 3, 4, 5}, nil, metrics.ProofValueIgnored},
		{"head off chain", offChainHead, []uint64{2, 3}, nil, metrics.ProofValueIgnored},
		{"target not after source", valueTestData(0, 0, 2), []uint64{2, 3}, nil, metrics.ProofValueIgnored},
		{"voters already counted", valueTestData(0, 1, 2), []uint64{0, 1}, nil, metrics.ProofValueNoNewVotes},
		{"new voter already held", valueTestData(0, 1, 2), []uint64{0, 2}, map[uint64]bool{2: true}, metrics.ProofValueNoNewVotes},
		{"one new voter, 3 of 6", valueTestData(0, 1, 2), []uint64{2}, nil, metrics.ProofValueAddsVotes},
		{"two new voters reach 4 of 6", valueTestData(0, 1, 2), []uint64{2, 3}, nil, metrics.ProofValueJustifies},
		{"held voters count toward 2/3", valueTestData(0, 1, 2), []uint64{2, 3}, map[uint64]bool{3: true}, metrics.ProofValueJustifies},
		{"untracked target, 3 of 6", valueTestData(0, 2, 2), []uint64{0, 1, 2}, nil, metrics.ProofValueAddsVotes},
		{"untracked target, 4 of 6", valueTestData(0, 2, 2), []uint64{0, 1, 2, 3}, nil, metrics.ProofValueJustifies},
		{"voter outside the registry", valueTestData(0, 1, 2), []uint64{9}, nil, metrics.ProofValueNoNewVotes},
		{"no data", nil, []uint64{2}, nil, metrics.ProofValueIgnored},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := proofValue(valueTestState(), tc.data, tc.voters, tc.held); got != tc.want {
				t.Fatalf("value = %q, want %q", got, tc.want)
			}
		})
	}
}

// valueTestSnapshot holds one group per entry, each voting for the slot-1
// target with the given raw signers; group 1 also has validator 2 in a held
// aggregate.
func valueTestSnapshot(signers ...[]uint64) *Snapshot {
	snap := &Snapshot{
		headState:    valueTestState(),
		attSigs:      make(map[[32]byte]*store.AttestationDataEntry),
		newEntries:   make(map[[32]byte]*store.PayloadEntry),
		knownEntries: make(map[[32]byte]*store.PayloadEntry),
	}
	for i, ids := range signers {
		data := valueTestData(0, 1, 2)
		data.Slot = uint64(10 + i) // distinct data per group
		entry := &store.AttestationDataEntry{Data: data}
		for _, id := range ids {
			entry.Signatures = append(entry.Signatures, store.AttestationSignatureEntry{ValidatorID: id})
		}
		snap.attSigs[rootByte(byte(i+1))] = entry
	}
	snap.newEntries[rootByte(1)] = &store.PayloadEntry{
		Data:   snap.attSigs[rootByte(1)].Data,
		Proofs: []*types.SingleMessageAggregate{{Participants: types.BitlistFromIndices([]uint64{2}), Proof: []byte{1}}},
	}
	return snap
}

func TestDeferredValuesUseEverySignerOnHand(t *testing.T) {
	snap := valueTestSnapshot([]uint64{2}, []uint64{0, 1}, []uint64{2, 3})
	groups := []aggregationGroup{{dataRoot: rootByte(1)}, {dataRoot: rootByte(2)}, {dataRoot: rootByte(3)}}
	got := deferredValues(snap, groups)
	want := map[string]int{
		metrics.ProofValueNoNewVotes: 2, // group 1: its only signer is held; group 2: both already counted
		metrics.ProofValueJustifies:  1, // group 3: 0,1 counted + 2,3 new = 4 of 6
	}
	if len(got) != len(want) {
		t.Fatalf("values = %v, want %v", got, want)
	}
	for k, n := range want {
		if got[k] != n {
			t.Fatalf("values = %v, want %v", got, want)
		}
	}
}

// A validator that is both held and a raw signer is one voter. Counted twice, a
// group one vote short of 2/3 would read as justifying its target.
func TestDeferredValuesCountAVoterOnce(t *testing.T) {
	snap := valueTestSnapshot([]uint64{2, 3})
	tally := types.NewBitlistSSZ(valueTestValidators)
	types.BitlistSet(tally, 0) // only validator 0 counted on chain
	snap.headState.JustificationsValidators = tally
	// 0 counted + 2 (held and raw) + 3 = 3 of 6, one short of 2/3.
	got := deferredValues(snap, []aggregationGroup{{dataRoot: rootByte(1)}})
	if got[metrics.ProofValueAddsVotes] != 1 {
		t.Fatalf("values = %v, want one adds_votes", got)
	}
}

// The session counts each proof it makes and each group it stops before, and
// never the same group twice.
func TestSessionCountsProofAndDeferredValues(t *testing.T) {
	snap := valueTestSnapshot([]uint64{3, 4}, []uint64{2, 3}, []uint64{0, 1}, []uint64{4, 5})
	cache := xmss.NewPubKeyCache()
	defer cache.Close()
	prove := func([]xmss.CPubKey, []xmss.CSig, []xmss.ChildProof, [32]byte, uint32) ([]byte, error) {
		return []byte{1}, nil
	}
	proofsBefore := counterSum(t, "lean_aggregation_proof_value_total")
	deferredBefore := counterSum(t, "lean_aggregation_deferred_group_value_total")

	aggs, _, _, truncated, _ := aggregateFromSnapshotWithProver(nil, snap, cache,
		time.Now().Add(time.Hour), MaxGroupsPerSession, shadow.Rates{}, newUnitCostEstimator(), prove)

	if !truncated || len(aggs) != MaxGroupsPerSession {
		t.Fatalf("aggs=%d truncated=%v, want the cap to stop the session", len(aggs), truncated)
	}
	if n := counterSum(t, "lean_aggregation_proof_value_total") - proofsBefore; n != float64(len(aggs)) {
		t.Fatalf("proofs counted = %v, want %d", n, len(aggs))
	}
	if n := counterSum(t, "lean_aggregation_deferred_group_value_total") - deferredBefore; n != float64(4-len(aggs)) {
		t.Fatalf("deferred groups counted = %v, want %d", n, 4-len(aggs))
	}
}

// counterSum adds every series of a counter in the default registry.
func counterSum(t *testing.T, name string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	sum := 0.0
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, m := range family.GetMetric() {
			sum += m.GetCounter().GetValue()
		}
	}
	return sum
}
