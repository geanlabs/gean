package unit

import (
	"context"
	"testing"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/network/gossipsub"
	"github.com/geanlabs/gean/node"
	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/storage/memory"
	"github.com/geanlabs/gean/types"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

func TestValidatorDuties_TryAttest_SignsAndPublishes(t *testing.T) {
	// Setup
	numValidators := uint64(3)
	state := statetransition.GenerateGenesis(1000, makeTestValidators(numValidators))
	emptyBody := &types.BlockBody{Attestations: []*types.Attestation{}}
	genesisBlock := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash,
		Body:          emptyBody,
	}
	stateRoot, _ := state.HashTreeRoot()
	genesisBlock.StateRoot = stateRoot

	store := memory.New()
	fc := forkchoice.NewStore(state, genesisBlock, store)

	// Mock keys
	keys := make(map[uint64]forkchoice.Signer)
	expectedSig := make([]byte, types.XMSSSignatureSize)
	expectedSig[0] = 0xAA // Marker
	keys[1] = &testSigner{sig: expectedSig}

	// Capture published attestation
	var publishedAtt *types.SignedAttestation
	publishFunc := func(ctx context.Context, topic *pubsub.Topic, sa *types.SignedAttestation) error {
		publishedAtt = sa
		return nil
	}

	duties := &node.ValidatorDuties{
		Indices:            []uint64{1},
		Keys:               keys,
		FC:                 fc,
		Topics:             &gossipsub.Topics{Attestation: &pubsub.Topic{}}, // Dummy topic
		PublishAttestation: publishFunc,
		Log:                logging.NewComponentLogger(logging.CompValidator),
	}

	// Action: validator 1 attests at slot 0
	duties.TryAttest(context.Background(), 0)

	// Verify
	if publishedAtt == nil {
		t.Fatal("expected PublishAttestation to be called")
	}
	if publishedAtt.Message.ValidatorID != 1 {
		t.Errorf("attester = %d, want 1", publishedAtt.Message.ValidatorID)
	}
	// Verify signature
	if publishedAtt.Signature[0] != 0xAA {
		t.Errorf("signature not matching mock signer output")
	}
}

func TestValidatorDuties_TryPropose_SignsAndPublishes(t *testing.T) {
	// Setup
	numValidators := uint64(3)
	state := statetransition.GenerateGenesis(1000, makeTestValidators(numValidators))
	emptyBody := &types.BlockBody{Attestations: []*types.Attestation{}}
	genesisBlock := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash,
		Body:          emptyBody,
	}
	stateRoot, _ := state.HashTreeRoot()
	genesisBlock.StateRoot = stateRoot

	store := memory.New()
	fc := forkchoice.NewStore(state, genesisBlock, store)

	// Mock keys
	keys := make(map[uint64]forkchoice.Signer)
	expectedSig := make([]byte, types.XMSSSignatureSize)
	expectedSig[0] = 0xBB // Marker
	keys[1] = &testSigner{sig: expectedSig}

	// Capture published block
	var publishedBlock *types.SignedBlockWithAttestation
	publishFunc := func(ctx context.Context, topic *pubsub.Topic, sb *types.SignedBlockWithAttestation) error {
		publishedBlock = sb
		return nil
	}

	duties := &node.ValidatorDuties{
		Indices:      []uint64{1},
		Keys:         keys,
		FC:           fc,
		Topics:       &gossipsub.Topics{Block: &pubsub.Topic{}}, // Dummy topic
		PublishBlock: publishFunc,
		Log:          logging.NewComponentLogger(logging.CompValidator),
	}

	// Action: validator 1 proposes at slot 1
	// 3 validators. Proposer = slot % 3. 1 % 3 = 1. Yes.
	duties.TryPropose(context.Background(), 1)

	// Verify
	if publishedBlock == nil {
		t.Fatal("expected PublishBlock to be called")
	}
	if publishedBlock.Message.Block.ProposerIndex != 1 {
		t.Errorf("proposer = %d, want 1", publishedBlock.Message.Block.ProposerIndex)
	}

	// Verify signature at last index (proposer sig is set by ProduceBlock).
	lastIdx := len(publishedBlock.Signature) - 1
	if publishedBlock.Signature[lastIdx][0] != 0xBB {
		t.Errorf("signature not matching mock signer output")
	}
}

// Helpers
func makeTestValidators(n uint64) []*types.Validator {
	vals := make([]*types.Validator, n)
	for i := uint64(0); i < n; i++ {
		vals[i] = &types.Validator{
			Pubkey: [52]byte{},
			Index:  i,
		}
	}
	return vals
}
