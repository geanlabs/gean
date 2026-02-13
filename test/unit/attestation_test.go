package unit

import (
	"testing"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/types"
)

// buildChainState produces consecutive blocks from genesis through targetSlot
// (inclusive), returning the final state and a map of slot -> block hash.
func buildChainState(t *testing.T, numValidators, targetSlot uint64) (*types.State, map[uint64][32]byte) {
	t.Helper()

	state := statetransition.GenerateGenesis(1000, makeTestValidators(numValidators))
	blockHashes := make(map[uint64][32]byte)

	emptyBody := &types.BlockBody{Attestations: []*types.Attestation{}}
	genesisBlock := &types.Block{
		Slot:          0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash,
		Body:          emptyBody,
		ProposerIndex: 0,
	}
	stateRoot, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("genesis state root: %v", err)
	}
	genesisBlock.StateRoot = stateRoot
	genesisHash, err := genesisBlock.HashTreeRoot()
	if err != nil {
		t.Fatalf("genesis block root: %v", err)
	}
	blockHashes[0] = genesisHash

	for slot := uint64(1); slot <= targetSlot; slot++ {
		proposer := slot % numValidators
		advanced, err := statetransition.ProcessSlots(state, slot)
		if err != nil {
			t.Fatalf("process slots(%d): %v", slot, err)
		}
		parentRoot, err := advanced.LatestBlockHeader.HashTreeRoot()
		if err != nil {
			t.Fatalf("parent root(%d): %v", slot, err)
		}

		block := &types.Block{
			Slot:          slot,
			ProposerIndex: proposer,
			ParentRoot:    parentRoot,
			StateRoot:     types.ZeroHash,
			Body:          &types.BlockBody{Attestations: []*types.Attestation{}},
		}
		postState, err := statetransition.ProcessBlock(advanced, block)
		if err != nil {
			t.Fatalf("process block(%d): %v", slot, err)
		}
		sr, err := postState.HashTreeRoot()
		if err != nil {
			t.Fatalf("post state root(%d): %v", slot, err)
		}
		block.StateRoot = sr

		state, err = statetransition.StateTransition(state, block)
		if err != nil {
			t.Fatalf("state transition(%d): %v", slot, err)
		}

		bh, err := block.HashTreeRoot()
		if err != nil {
			t.Fatalf("block hash(%d): %v", slot, err)
		}
		blockHashes[slot] = bh
	}

	return state, blockHashes
}

func makeAttestation(validatorID uint64, source, target *types.Checkpoint) *types.Attestation {
	return &types.Attestation{
		ValidatorID: validatorID,
		Data: &types.AttestationData{
			Slot:   target.Slot,
			Head:   target,
			Target: target,
			Source: source,
		},
	}
}

// makeSupermajorityAttestations creates attestations from enough validators
// to reach supermajority (4 out of 5 for numValidators=5).
func makeSupermajorityAttestations(numValidators uint64, source, target *types.Checkpoint) []*types.Attestation {
	// Need 3*count >= 2*numValidators, so count >= ceil(2*numValidators/3).
	needed := (2*numValidators + 2) / 3
	atts := make([]*types.Attestation, needed)
	for i := uint64(0); i < needed; i++ {
		atts[i] = makeAttestation(i, source, target)
	}
	return atts
}

func TestAttestationJustifiesTargetWithSupermajority(t *testing.T) {
	state, _ := buildChainState(t, 5, 2)

	source := &types.Checkpoint{Root: state.HistoricalBlockHashes[0], Slot: 0}
	target := &types.Checkpoint{Root: state.HistoricalBlockHashes[1], Slot: 1}

	atts := makeSupermajorityAttestations(5, source, target)
	next := statetransition.ProcessAttestations(state, atts)

	if next.LatestJustified.Slot != 1 {
		t.Fatalf("latest justified slot = %d, want 1", next.LatestJustified.Slot)
	}
}

func TestAttestationBelowSupermajorityDoesNotJustify(t *testing.T) {
	state, _ := buildChainState(t, 5, 2)

	source := &types.Checkpoint{Root: state.HistoricalBlockHashes[0], Slot: 0}
	target := &types.Checkpoint{Root: state.HistoricalBlockHashes[1], Slot: 1}

	// Only 3 out of 5 — supermajority needs 4.
	atts := []*types.Attestation{
		makeAttestation(0, source, target),
		makeAttestation(1, source, target),
		makeAttestation(2, source, target),
	}
	next := statetransition.ProcessAttestations(state, atts)

	if next.LatestJustified.Slot != state.LatestJustified.Slot {
		t.Fatalf("should not justify with only 3/5 validators: got justified slot %d", next.LatestJustified.Slot)
	}

	// Votes should be tracked in justifications_roots/validators.
	if len(next.JustificationsRoots) != 1 {
		t.Fatalf("expected 1 pending justification root, got %d", len(next.JustificationsRoots))
	}
}

func TestAttestationIgnoredWhenSourceNotJustified(t *testing.T) {
	state, _ := buildChainState(t, 5, 2)

	// Slot 1 is not justified.
	source := &types.Checkpoint{Root: state.HistoricalBlockHashes[1], Slot: 1}
	target := &types.Checkpoint{Root: state.HistoricalBlockHashes[1], Slot: 1} // target == source to also fail ordering

	next := statetransition.ProcessAttestations(state, makeSupermajorityAttestations(5, source, target))

	if next.LatestJustified.Slot != state.LatestJustified.Slot {
		t.Fatalf("latest justified slot changed: got %d want %d", next.LatestJustified.Slot, state.LatestJustified.Slot)
	}
}

func TestAttestationIgnoredWhenSourceIsNotBeforeTarget(t *testing.T) {
	state, _ := buildChainState(t, 5, 2)

	source := &types.Checkpoint{Root: state.HistoricalBlockHashes[0], Slot: 0}
	target := &types.Checkpoint{Root: state.HistoricalBlockHashes[0], Slot: 0} // same as source

	next := statetransition.ProcessAttestations(state, makeSupermajorityAttestations(5, source, target))

	if next.LatestJustified.Slot != state.LatestJustified.Slot {
		t.Fatalf("latest justified slot changed: got %d want %d", next.LatestJustified.Slot, state.LatestJustified.Slot)
	}
}

func TestAttestationIgnoredWhenRootMismatch(t *testing.T) {
	state, _ := buildChainState(t, 5, 2)

	source := &types.Checkpoint{Root: state.HistoricalBlockHashes[0], Slot: 0}
	target := &types.Checkpoint{Root: [32]byte{0xff}, Slot: 1} // wrong root

	next := statetransition.ProcessAttestations(state, makeSupermajorityAttestations(5, source, target))

	if next.LatestJustified.Slot != state.LatestJustified.Slot {
		t.Fatalf("should not justify with wrong target root")
	}
}

func TestConsecutiveJustificationFinalizes(t *testing.T) {
	state, _ := buildChainState(t, 5, 3)

	// Step 1: Justify slot 1 from genesis (slot 0).
	source0 := &types.Checkpoint{Root: state.HistoricalBlockHashes[0], Slot: 0}
	target1 := &types.Checkpoint{Root: state.HistoricalBlockHashes[1], Slot: 1}
	state = statetransition.ProcessAttestations(state, makeSupermajorityAttestations(5, source0, target1))

	if state.LatestJustified.Slot != 1 {
		t.Fatalf("step 1: latest justified = %d, want 1", state.LatestJustified.Slot)
	}

	// Step 2: Justify slot 2 from slot 1. Slots 0→1→2 are consecutive with
	// no justifiable gap, so slot 1 (source) should be finalized.
	source1 := &types.Checkpoint{Root: state.HistoricalBlockHashes[1], Slot: 1}
	target2 := &types.Checkpoint{Root: state.HistoricalBlockHashes[2], Slot: 2}
	state = statetransition.ProcessAttestations(state, makeSupermajorityAttestations(5, source1, target2))

	if state.LatestJustified.Slot != 2 {
		t.Fatalf("step 2: latest justified = %d, want 2", state.LatestJustified.Slot)
	}
	if state.LatestFinalized.Slot != 1 {
		t.Fatalf("step 2: latest finalized = %d, want 1", state.LatestFinalized.Slot)
	}
}

func TestVoteTrackingSurvivesRoundTrip(t *testing.T) {
	state, _ := buildChainState(t, 5, 2)

	source := &types.Checkpoint{Root: state.HistoricalBlockHashes[0], Slot: 0}
	target := &types.Checkpoint{Root: state.HistoricalBlockHashes[1], Slot: 1}

	// 3 votes: not enough for supermajority but should be tracked.
	atts := []*types.Attestation{
		makeAttestation(0, source, target),
		makeAttestation(1, source, target),
		makeAttestation(2, source, target),
	}
	next := statetransition.ProcessAttestations(state, atts)

	// Add 1 more vote in a second pass to reach supermajority (4/5).
	next = statetransition.ProcessAttestations(next, []*types.Attestation{
		makeAttestation(3, source, target),
	})

	if next.LatestJustified.Slot != 1 {
		t.Fatalf("should justify after accumulating 4/5 votes across passes, got justified slot %d", next.LatestJustified.Slot)
	}
}

func TestDuplicateVoteNotDoubleCounted(t *testing.T) {
	state, _ := buildChainState(t, 5, 2)

	source := &types.Checkpoint{Root: state.HistoricalBlockHashes[0], Slot: 0}
	target := &types.Checkpoint{Root: state.HistoricalBlockHashes[1], Slot: 1}

	// Same validator votes 4 times — should count as 1.
	atts := []*types.Attestation{
		makeAttestation(0, source, target),
		makeAttestation(0, source, target),
		makeAttestation(0, source, target),
		makeAttestation(0, source, target),
	}
	next := statetransition.ProcessAttestations(state, atts)

	if next.LatestJustified.Slot != state.LatestJustified.Slot {
		t.Fatalf("duplicate votes should not justify: got justified slot %d", next.LatestJustified.Slot)
	}
}

func TestOriginalStateNotMutated(t *testing.T) {
	state, _ := buildChainState(t, 5, 2)

	origJustifiedSlot := state.LatestJustified.Slot
	origRootsLen := len(state.JustificationsRoots)
	origValsBytes := make([]byte, len(state.JustificationsValidators))
	copy(origValsBytes, state.JustificationsValidators)

	source := &types.Checkpoint{Root: state.HistoricalBlockHashes[0], Slot: 0}
	target := &types.Checkpoint{Root: state.HistoricalBlockHashes[1], Slot: 1}
	_ = statetransition.ProcessAttestations(state, makeSupermajorityAttestations(5, source, target))

	// Original state must not be modified.
	if state.LatestJustified.Slot != origJustifiedSlot {
		t.Fatal("original state LatestJustified was mutated")
	}
	if len(state.JustificationsRoots) != origRootsLen {
		t.Fatal("original state JustificationsRoots was mutated")
	}
	for i, b := range state.JustificationsValidators {
		if b != origValsBytes[i] {
			t.Fatal("original state JustificationsValidators was mutated")
		}
	}
}
