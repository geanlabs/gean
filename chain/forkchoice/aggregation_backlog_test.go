package forkchoice

import (
	"testing"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/storage/memory"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss/leansig"
)

const aggregationTestActiveEpochs = 8

func TestAggregateCommitteeSignatures_CurrentSlotOnly(t *testing.T) {
	fc, signers, cleanup := newAggregationTestStore(t, 2)
	defer cleanup()

	currentA, err := fc.ProduceAttestation(1, 0, signers[0])
	if err != nil {
		t.Fatalf("produce attestation validator 0: %v", err)
	}
	currentB, err := fc.ProduceAttestation(1, 1, signers[1])
	if err != nil {
		t.Fatalf("produce attestation validator 1: %v", err)
	}

	stale := cloneSignedAttestationWithSlot(currentA, 0)
	future := cloneSignedAttestationWithSlot(currentA, 2)

	fc.mu.Lock()
	fc.storeGossipSignatureLocked(currentA)
	fc.storeGossipSignatureLocked(currentB)
	fc.storeGossipSignatureLocked(stale)
	fc.storeGossipSignatureLocked(future)
	fc.mu.Unlock()

	aggregated, err := fc.AggregateCommitteeSignatures()
	if err != nil {
		t.Fatalf("AggregateCommitteeSignatures returned error: %v", err)
	}
	if len(aggregated) != 1 {
		t.Fatalf("aggregated count = %d, want 1", len(aggregated))
	}
	if aggregated[0].Data == nil || aggregated[0].Data.Slot != 1 {
		t.Fatalf("aggregated attestation slot = %v, want 1", aggregated[0].Data)
	}
	if got := len(bitlistToValidatorIDs(aggregated[0].Proof.Participants)); got != 2 {
		t.Fatalf("participants = %d, want 2", got)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.gossipSignatures) != 1 {
		t.Fatalf("remaining gossip signatures = %d, want 1 future entry", len(fc.gossipSignatures))
	}
	for _, stored := range fc.gossipSignatures {
		if stored.data == nil || stored.data.Slot != 2 {
			t.Fatalf("remaining gossip signature slot = %v, want 2", stored.data)
		}
	}
}

func TestAggregateCommitteeSignatures_NoCurrentSlotEligible(t *testing.T) {
	fc, signers, cleanup := newAggregationTestStore(t, 1)
	defer cleanup()

	current, err := fc.ProduceAttestation(1, 0, signers[0])
	if err != nil {
		t.Fatalf("produce attestation: %v", err)
	}

	stale := cloneSignedAttestationWithSlot(current, 0)
	future := cloneSignedAttestationWithSlot(current, 2)

	fc.mu.Lock()
	fc.storeGossipSignatureLocked(stale)
	fc.storeGossipSignatureLocked(future)
	fc.mu.Unlock()

	aggregated, err := fc.AggregateCommitteeSignatures()
	if err != nil {
		t.Fatalf("AggregateCommitteeSignatures returned error: %v", err)
	}
	if len(aggregated) != 0 {
		t.Fatalf("aggregated count = %d, want 0", len(aggregated))
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.gossipSignatures) != 1 {
		t.Fatalf("remaining gossip signatures = %d, want 1 future entry", len(fc.gossipSignatures))
	}
	for _, stored := range fc.gossipSignatures {
		if stored.data == nil || stored.data.Slot != 2 {
			t.Fatalf("remaining gossip signature slot = %v, want 2", stored.data)
		}
	}
}

func newAggregationTestStore(t *testing.T, numValidators int) (*Store, []*leansig.Keypair, func()) {
	t.Helper()

	validators := make([]*types.Validator, numValidators)
	signers := make([]*leansig.Keypair, numValidators)
	for i := 0; i < numValidators; i++ {
		kp, err := leansig.GenerateKeypair(uint64(101+i), 0, aggregationTestActiveEpochs)
		if err != nil {
			t.Fatalf("generate keypair %d: %v", i, err)
		}
		pkBytes, err := kp.PublicKeyBytes()
		if err != nil {
			kp.Free()
			t.Fatalf("serialize pubkey %d: %v", i, err)
		}
		var pubkey [52]byte
		if len(pkBytes) != len(pubkey) {
			kp.Free()
			t.Fatalf("pubkey length = %d, want 52", len(pkBytes))
		}

		copy(pubkey[:], pkBytes)
		validators[i] = &types.Validator{
			Index:  uint64(i),
			Pubkey: pubkey,
		}
		signers[i] = kp
	}

	state := statetransition.GenerateGenesis(1000, validators)
	emptyBody := &types.BlockBody{Attestations: []*types.AggregatedAttestation{}}
	genesisBlock := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash,
		Body:          emptyBody,
	}
	stateRoot, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash genesis state: %v", err)
	}
	genesisBlock.StateRoot = stateRoot

	fc := NewStore(state, genesisBlock, memory.New())
	fc.SetIsAggregator(true)

	cleanup := func() {
		for _, kp := range signers {
			if kp != nil {
				kp.Free()
			}
		}
	}
	return fc, signers, cleanup
}

func cloneSignedAttestationWithSlot(sa *types.SignedAttestation, slot uint64) *types.SignedAttestation {
	cloned := *sa
	if sa.Message != nil {
		msg := *sa.Message
		msg.Slot = slot
		cloned.Message = &msg
	}
	return &cloned
}
