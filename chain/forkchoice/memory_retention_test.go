package forkchoice

import (
	"testing"

	"github.com/geanlabs/gean/storage/memory"
	"github.com/geanlabs/gean/types"
)

func newCheckpointTestStore(t *testing.T) (*Store, [32]byte, *types.State) {
	t.Helper()

	state := makeCheckpointState()
	anchorRoot := prepareCheckpointStateForStore(t, state)
	return NewStoreFromCheckpointState(state, anchorRoot, memory.New()), anchorRoot, state
}

func makeKnownAttestation(anchorRoot [32]byte, state *types.State) *types.SignedAttestation {
	return &types.SignedAttestation{
		ValidatorID: 0,
		Message: &types.AttestationData{
			Slot:   state.Slot,
			Head:   &types.Checkpoint{Root: anchorRoot, Slot: state.Slot},
			Source: &types.Checkpoint{Root: state.LatestJustified.Root, Slot: state.LatestJustified.Slot},
			Target: &types.Checkpoint{Root: anchorRoot, Slot: state.Slot},
		},
		Signature: [types.XMSSSignatureSize]byte{0x01},
	}
}

func makeAttestationData(anchorRoot [32]byte, state *types.State, slot uint64) *types.AttestationData {
	return &types.AttestationData{
		Slot:   slot,
		Head:   &types.Checkpoint{Root: anchorRoot, Slot: state.Slot},
		Source: &types.Checkpoint{Root: state.LatestJustified.Root, Slot: state.LatestJustified.Slot},
		Target: &types.Checkpoint{Root: anchorRoot, Slot: state.Slot},
	}
}

func TestProcessAttestationLocked_NonAggregatorDoesNotCacheGossipSignature(t *testing.T) {
	fc, anchorRoot, state := newCheckpointTestStore(t)
	sa := makeKnownAttestation(anchorRoot, state)

	fc.mu.Lock()
	fc.processAttestationLocked(sa, false)
	fc.mu.Unlock()

	if got := len(fc.gossipSignatures); got != 0 {
		t.Fatalf("expected no cached gossip signatures on non-aggregator, got %d", got)
	}
}

func TestProcessAttestationLocked_AggregatorCachesGossipSignature(t *testing.T) {
	fc, anchorRoot, state := newCheckpointTestStore(t)
	fc.isAggregator = true
	sa := makeKnownAttestation(anchorRoot, state)

	fc.mu.Lock()
	fc.processAttestationLocked(sa, false)
	fc.mu.Unlock()

	if got := len(fc.gossipSignatures); got != 1 {
		t.Fatalf("expected 1 cached gossip signature on aggregator, got %d", got)
	}
}

func TestProcessAttestationLocked_OnChainVoteDoesNotCacheGossipSignature(t *testing.T) {
	fc, anchorRoot, state := newCheckpointTestStore(t)
	fc.isAggregator = true
	sa := makeKnownAttestation(anchorRoot, state)

	fc.mu.Lock()
	fc.processAttestationLocked(sa, true)
	fc.mu.Unlock()

	if got := len(fc.gossipSignatures); got != 0 {
		t.Fatalf("expected no cached gossip signatures for on-chain vote, got %d", got)
	}
}

func TestPruneEphemeralCaches_RemovesFinalizedEntries(t *testing.T) {
	fc, anchorRoot, state := newCheckpointTestStore(t)

	oldData := makeAttestationData(anchorRoot, state, 1)
	newData := makeAttestationData(anchorRoot, state, 4)
	oldKey, ok := makeSignatureKey(0, oldData)
	if !ok {
		t.Fatal("expected old signature key")
	}
	newKey, ok := makeSignatureKey(1, newData)
	if !ok {
		t.Fatal("expected new signature key")
	}
	oldDataRoot, err := oldData.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash old data root: %v", err)
	}
	newDataRoot, err := newData.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash new data root: %v", err)
	}

	oldProof := &types.AggregatedSignatureProof{Participants: []byte{0x01}, ProofData: []byte{0xAA}}
	newProof := &types.AggregatedSignatureProof{Participants: []byte{0x01}, ProofData: []byte{0xBB}}

	fc.gossipSignatures[oldKey] = storedSignature{slot: 1, data: oldData}
	fc.gossipSignatures[newKey] = storedSignature{slot: 4, data: newData}
	fc.aggregatedPayloads[oldKey] = []storedAggregatedPayload{{slot: 1, proof: oldProof}}
	fc.aggregatedPayloads[newKey] = []storedAggregatedPayload{{slot: 4, proof: newProof}}
	fc.latestKnownAggregatedPayloads[oldDataRoot] = aggregatedPayload{data: oldData, proofs: []*types.AggregatedSignatureProof{oldProof}}
	fc.latestKnownAggregatedPayloads[newDataRoot] = aggregatedPayload{data: newData, proofs: []*types.AggregatedSignatureProof{newProof}}
	fc.latestNewAggregatedPayloads[oldDataRoot] = aggregatedPayload{data: oldData, proofs: []*types.AggregatedSignatureProof{oldProof}}
	fc.latestNewAggregatedPayloads[newDataRoot] = aggregatedPayload{data: newData, proofs: []*types.AggregatedSignatureProof{newProof}}

	fc.PruneEphemeralCaches(2)

	if got := len(fc.gossipSignatures); got != 1 {
		t.Fatalf("expected 1 gossip signature after prune, got %d", got)
	}
	if _, ok := fc.gossipSignatures[oldKey]; ok {
		t.Fatal("expected finalized gossip signature to be pruned")
	}
	if _, ok := fc.gossipSignatures[newKey]; !ok {
		t.Fatal("expected post-finalized gossip signature to remain")
	}

	if got := len(fc.aggregatedPayloads); got != 1 {
		t.Fatalf("expected 1 aggregated payload key after prune, got %d", got)
	}
	if _, ok := fc.aggregatedPayloads[oldKey]; ok {
		t.Fatal("expected finalized aggregated payload key to be pruned")
	}
	if _, ok := fc.aggregatedPayloads[newKey]; !ok {
		t.Fatal("expected post-finalized aggregated payload key to remain")
	}

	if got := len(fc.latestKnownAggregatedPayloads); got != 1 {
		t.Fatalf("expected 1 known aggregated payload root after prune, got %d", got)
	}
	if _, ok := fc.latestKnownAggregatedPayloads[oldDataRoot]; ok {
		t.Fatal("expected finalized known aggregated payload root to be pruned")
	}
	if _, ok := fc.latestKnownAggregatedPayloads[newDataRoot]; !ok {
		t.Fatal("expected post-finalized known aggregated payload root to remain")
	}

	if got := len(fc.latestNewAggregatedPayloads); got != 1 {
		t.Fatalf("expected 1 new aggregated payload root after prune, got %d", got)
	}
	if _, ok := fc.latestNewAggregatedPayloads[oldDataRoot]; ok {
		t.Fatal("expected finalized new aggregated payload root to be pruned")
	}
	if _, ok := fc.latestNewAggregatedPayloads[newDataRoot]; !ok {
		t.Fatal("expected post-finalized new aggregated payload root to remain")
	}
}
