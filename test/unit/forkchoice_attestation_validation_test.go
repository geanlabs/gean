package unit

import (
	"testing"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/xmss/leansig"
	"github.com/geanlabs/gean/types"
)

func buildForkChoiceWithBlocks(t *testing.T, numValidators, targetSlot uint64) (*forkchoice.Store, map[uint64][32]byte) {
	t.Helper()

	fc, state := makeGenesisFC(numValidators)
	blockHashes := map[uint64][32]byte{0: fc.Head}

	for slot := uint64(1); slot <= targetSlot; slot++ {
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
			ProposerIndex: slot % numValidators,
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
			t.Fatalf("post-state root(%d): %v", slot, err)
		}
		block.StateRoot = sr

		state, err = statetransition.StateTransition(state, block)
		if err != nil {
			t.Fatalf("state transition(%d): %v", slot, err)
		}

		envelope := &types.SignedBlockWithAttestation{
			Message: &types.BlockWithAttestation{Block: block},
		}
		if err := fc.ProcessBlock(envelope); err != nil {
			t.Fatalf("forkchoice process block(%d): %v", slot, err)
		}
		bh, err := block.HashTreeRoot()
		if err != nil {
			t.Fatalf("block hash(%d): %v", slot, err)
		}
		blockHashes[slot] = bh
	}

	return fc, blockHashes
}

func makeFCAttestation(validatorID, slot uint64, head, source, target *types.Checkpoint) *types.SignedAttestation {
	return &types.SignedAttestation{
		Message: &types.Attestation{
			ValidatorID: validatorID,
			Data: &types.AttestationData{
				Slot:   slot,
				Head:   head,
				Target: target,
				Source: source,
			},
		},
	}
}

func TestForkChoiceProcessAttestationValidGossip(t *testing.T) {
	// Use a real keypair so the attestation signature is valid.
	kp, err := leansig.GenerateKeypair(42, 0, 8)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	defer kp.Free()

	pubkey, err := kp.PublicKeyBytes()
	if err != nil {
		t.Fatalf("pubkey: %v", err)
	}
	var pk [52]byte
	copy(pk[:], pubkey)

	fc, _ := buildForkChoiceWithBlocks(t, 5, 2)

	// Patch validator 0's pubkey in all stored states so signature verification
	// can succeed.
	for _, st := range fc.Storage.GetAllStates() {
		st.Validators[0].Pubkey = pk
	}

	fc.Time = 10 * types.IntervalsPerSlot // current slot far ahead of vote slot

	// Produce a properly signed attestation for validator 0 at slot 2.
	sa, err := fc.ProduceAttestation(2, 0, kp)
	if err != nil {
		t.Fatalf("ProduceAttestation: %v", err)
	}
	fc.ProcessAttestation(sa)

	got, ok := fc.LatestNewAttestations[0]
	if !ok {
		t.Fatal("expected validator attestation in latest_new_attestations")
	}
	if got.Message.Data.Target.Slot != sa.Message.Data.Target.Slot {
		t.Fatalf("unexpected attestation target slot: got %d, want %d", got.Message.Data.Target.Slot, sa.Message.Data.Target.Slot)
	}
}

func TestForkChoiceProcessAttestationRejectsCheckpointSlotMismatch(t *testing.T) {
	fc, hashes := buildForkChoiceWithBlocks(t, 5, 2)
	fc.Time = 10 * types.IntervalsPerSlot

	sa := makeFCAttestation(1, 2,
		&types.Checkpoint{Root: hashes[2], Slot: 2},
		&types.Checkpoint{Root: hashes[1], Slot: 0}, // mismatch: block slot is 1
		&types.Checkpoint{Root: hashes[2], Slot: 2},
	)
	fc.ProcessAttestation(sa)

	if len(fc.LatestNewAttestations) != 0 {
		t.Fatalf("expected no new votes, got %d", len(fc.LatestNewAttestations))
	}
}

func TestForkChoiceProcessAttestationRejectsTooFarFuture(t *testing.T) {
	fc, hashes := buildForkChoiceWithBlocks(t, 5, 2)
	fc.Time = 2 * types.IntervalsPerSlot // current slot = 2

	sa := makeFCAttestation(2, 4, // > currentSlot + 1
		&types.Checkpoint{Root: hashes[2], Slot: 2},
		&types.Checkpoint{Root: hashes[1], Slot: 1},
		&types.Checkpoint{Root: hashes[2], Slot: 2},
	)
	fc.ProcessAttestation(sa)

	if len(fc.LatestNewAttestations) != 0 {
		t.Fatalf("expected no new votes, got %d", len(fc.LatestNewAttestations))
	}
}

func TestForkChoiceProcessAttestationRejectsFutureGossipVote(t *testing.T) {
	fc, hashes := buildForkChoiceWithBlocks(t, 5, 2)
	fc.Time = 2 * types.IntervalsPerSlot // current slot = 2

	sa := makeFCAttestation(3, 3, // <= currentSlot+1 but > currentSlot, should fail gossip check
		&types.Checkpoint{Root: hashes[2], Slot: 2},
		&types.Checkpoint{Root: hashes[1], Slot: 1},
		&types.Checkpoint{Root: hashes[2], Slot: 2},
	)
	fc.ProcessAttestation(sa)

	if len(fc.LatestNewAttestations) != 0 {
		t.Fatalf("expected no new votes, got %d", len(fc.LatestNewAttestations))
	}
}
