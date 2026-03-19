package node_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/network/gossipsub"
	"github.com/geanlabs/gean/node"
	"github.com/geanlabs/gean/observability/logging"
	"github.com/geanlabs/gean/storage/bolt"
	"github.com/geanlabs/gean/types"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

type testSigner struct {
	sig []byte
}

func (s *testSigner) Sign(epoch uint32, message [32]byte) ([]byte, error) {
	if s.sig != nil {
		return s.sig, nil
	}
	out := make([]byte, 3112)
	out[0] = 0xAA
	return out, nil
}

func TestValidatorDuties_TryAttest_SignsAndPublishes(t *testing.T) {
	// Setup
	numValidators := uint64(3)
	state := statetransition.GenerateGenesis(1000, makeTestValidators(numValidators))
	emptyBody := &types.BlockBody{Attestations: []*types.AggregatedAttestation{}}
	genesisBlock := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash,
		Body:          emptyBody,
	}
	stateRoot, _ := state.HashTreeRoot()
	genesisBlock.StateRoot = stateRoot

	store := newBoltStore(t)
	fc := forkchoice.NewStore(state, genesisBlock, store)

	// Mock keys
	keys := make(map[uint64]forkchoice.Signer)
	expectedSig := make([]byte, 3112)
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
		Topics:             &gossipsub.Topics{SubnetAttestation: &pubsub.Topic{}}, // Dummy topic
		PublishAttestation: publishFunc,
		Log:                logging.NewComponentLogger(logging.CompValidator),
	}

	// Action: validator 1 attests at slot 0
	duties.TryAttest(context.Background(), 0)

	// Verify
	if publishedAtt == nil {
		t.Fatal("expected PublishAttestation to be called")
	}
	if publishedAtt.ValidatorID != 1 {
		t.Errorf("attester = %d, want 1", publishedAtt.ValidatorID)
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
	emptyBody := &types.BlockBody{Attestations: []*types.AggregatedAttestation{}}
	genesisBlock := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash,
		Body:          emptyBody,
	}
	stateRoot, _ := state.HashTreeRoot()
	genesisBlock.StateRoot = stateRoot

	store := newBoltStore(t)
	fc := forkchoice.NewStore(state, genesisBlock, store)

	// Mock keys
	keys := make(map[uint64]forkchoice.Signer)
	expectedSig := make([]byte, 3112)
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

	if publishedBlock.Signature.ProposerSignature[0] != 0xBB {
		t.Errorf("signature not matching mock signer output")
	}

	// Proposed blocks must not be inserted into forkchoice storage until they are
	// processed through the normal block import path (gossip/reqresp/sync).
	blockRoot, _ := publishedBlock.Message.Block.HashTreeRoot()
	if _, ok := fc.GetSignedBlock(blockRoot); ok {
		t.Fatalf("unexpected pre-inserted proposed block %x", blockRoot)
	}
}

func TestValidatorDuties_TryPropose_DuplicateIndexProposesOncePerSlot(t *testing.T) {
	// Setup
	numValidators := uint64(3)
	state := statetransition.GenerateGenesis(1000, makeTestValidators(numValidators))
	emptyBody := &types.BlockBody{Attestations: []*types.AggregatedAttestation{}}
	genesisBlock := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash,
		Body:          emptyBody,
	}
	stateRoot, _ := state.HashTreeRoot()
	genesisBlock.StateRoot = stateRoot

	store := newBoltStore(t)
	fc := forkchoice.NewStore(state, genesisBlock, store)

	keys := make(map[uint64]forkchoice.Signer)
	expectedSig := make([]byte, 3112)
	expectedSig[0] = 0xCC
	keys[1] = &testSigner{sig: expectedSig}

	publishCount := 0
	publishFunc := func(ctx context.Context, topic *pubsub.Topic, sb *types.SignedBlockWithAttestation) error {
		publishCount++
		return nil
	}

	duties := &node.ValidatorDuties{
		Indices:      []uint64{1, 1},
		Keys:         keys,
		FC:           fc,
		Topics:       &gossipsub.Topics{Block: &pubsub.Topic{}},
		PublishBlock: publishFunc,
		Log:          logging.NewComponentLogger(logging.CompValidator),
	}

	// Action: slot 1 proposer should run once even if index appears twice.
	duties.TryPropose(context.Background(), 1)

	if publishCount != 1 {
		t.Fatalf("publish count = %d, want 1", publishCount)
	}
}

func newBoltStore(t testing.TB) *bolt.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "bolt.db")
	store, err := bolt.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create bolt store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
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
