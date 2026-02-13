package unit

import (
	"testing"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/storage/memory"
	"github.com/geanlabs/gean/types"
)

func makeGenesisFC(numValidators uint64) (*forkchoice.Store, *types.State) {
	state := statetransition.GenerateGenesis(1000, numValidators)

	emptyBody := &types.BlockBody{Attestations: []*types.SignedVote{}}
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
	return fc, state
}

func TestForkChoiceInitFromGenesis(t *testing.T) {
	fc, _ := makeGenesisFC(5)

	if fc.Head == types.ZeroHash {
		t.Error("head should not be zero after genesis")
	}
	if fc.LatestJustified.Root != types.ZeroHash {
		t.Error("genesis justified root should be zero")
	}
}

func TestForkChoiceTickInterval(t *testing.T) {
	fc, _ := makeGenesisFC(5)
	initialTime := fc.Time

	fc.TickInterval(false)
	if fc.Time != initialTime+1 {
		t.Errorf("time = %d, want %d", fc.Time, initialTime+1)
	}
}

func TestForkChoiceAcceptNewVotes(t *testing.T) {
	fc, _ := makeGenesisFC(5)

	// Add a vote to new votes.
	fc.LatestNewVotes[0] = &types.Checkpoint{Root: fc.Head, Slot: 0}

	fc.AcceptNewVotes()

	if len(fc.LatestNewVotes) != 0 {
		t.Error("new votes should be empty after accept")
	}
	if _, ok := fc.LatestKnownVotes[0]; !ok {
		t.Error("vote should be in known votes after accept")
	}
}

func TestForkChoiceInitPanicsOnAnchorStateRootMismatch(t *testing.T) {
	state := statetransition.GenerateGenesis(1000, 5)
	emptyBody := &types.BlockBody{Attestations: []*types.SignedVote{}}
	genesisBlock := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash, // intentionally wrong
		Body:          emptyBody,
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for anchor block/state root mismatch")
		}
	}()
	_ = forkchoice.NewStore(state, genesisBlock, memory.New())
}

func TestProduceAttestationVoteAcceptsNewVotesFirst(t *testing.T) {
	fc, _ := makeGenesisFC(5)
	fc.LatestNewVotes[3] = &types.Checkpoint{Root: fc.Head, Slot: 0}

	_ = fc.ProduceAttestationVote(1, 0)

	if len(fc.LatestNewVotes) != 0 {
		t.Fatalf("expected latest_new_votes to be drained, got %d entries", len(fc.LatestNewVotes))
	}
	if _, ok := fc.LatestKnownVotes[3]; !ok {
		t.Fatal("expected vote to be moved into latest_known_votes")
	}
}
