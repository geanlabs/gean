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
