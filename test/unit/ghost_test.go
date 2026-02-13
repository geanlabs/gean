package unit

import (
	"testing"

	"github.com/devylongs/gean/chain/forkchoice"
	"github.com/devylongs/gean/storage/memory"
	"github.com/devylongs/gean/types"
)

func makeBlock(slot, proposer uint64, parent [32]byte) *types.Block {
	return &types.Block{
		Slot:          slot,
		ProposerIndex: proposer,
		ParentRoot:    parent,
		StateRoot:     types.ZeroHash,
		Body:          &types.BlockBody{Attestations: []*types.SignedVote{}},
	}
}

func TestGetForkChoiceHeadSingleChain(t *testing.T) {
	store := memory.New()

	genesis := makeBlock(0, 0, types.ZeroHash)
	genesisRoot, _ := genesis.HashTreeRoot()
	store.PutBlock(genesisRoot, genesis)

	block1 := makeBlock(1, 1, genesisRoot)
	block1Root, _ := block1.HashTreeRoot()
	store.PutBlock(block1Root, block1)

	block2 := makeBlock(2, 2, block1Root)
	block2Root, _ := block2.HashTreeRoot()
	store.PutBlock(block2Root, block2)

	// Vote for block2.
	votes := map[uint64]*types.Checkpoint{
		0: {Root: block2Root, Slot: 2},
	}

	head := forkchoice.GetForkChoiceHead(store, genesisRoot, votes, 0)
	if head != block2Root {
		t.Errorf("expected head = block2, got %x", head[:4])
	}
}

func TestGetForkChoiceHeadNoVotes(t *testing.T) {
	store := memory.New()

	genesis := makeBlock(0, 0, types.ZeroHash)
	genesisRoot, _ := genesis.HashTreeRoot()
	store.PutBlock(genesisRoot, genesis)

	votes := map[uint64]*types.Checkpoint{}

	head := forkchoice.GetForkChoiceHead(store, genesisRoot, votes, 0)
	if head != genesisRoot {
		t.Errorf("expected head = genesis with no votes")
	}
}

func TestGetForkChoiceHeadTwoForks(t *testing.T) {
	store := memory.New()

	genesis := makeBlock(0, 0, types.ZeroHash)
	genesisRoot, _ := genesis.HashTreeRoot()
	store.PutBlock(genesisRoot, genesis)

	// Fork A
	blockA := makeBlock(1, 0, genesisRoot)
	blockARoot, _ := blockA.HashTreeRoot()
	store.PutBlock(blockARoot, blockA)

	// Fork B
	blockB := makeBlock(1, 1, genesisRoot)
	blockBRoot, _ := blockB.HashTreeRoot()
	store.PutBlock(blockBRoot, blockB)

	// 2 votes for A, 1 vote for B -> head should be A.
	votes := map[uint64]*types.Checkpoint{
		0: {Root: blockARoot, Slot: 1},
		1: {Root: blockARoot, Slot: 1},
		2: {Root: blockBRoot, Slot: 1},
	}

	head := forkchoice.GetForkChoiceHead(store, genesisRoot, votes, 0)
	if head != blockARoot {
		t.Errorf("expected head = blockA (more votes)")
	}
}

func TestGetForkChoiceHeadMinScore(t *testing.T) {
	store := memory.New()

	genesis := makeBlock(0, 0, types.ZeroHash)
	genesisRoot, _ := genesis.HashTreeRoot()
	store.PutBlock(genesisRoot, genesis)

	block1 := makeBlock(1, 0, genesisRoot)
	block1Root, _ := block1.HashTreeRoot()
	store.PutBlock(block1Root, block1)

	// Only 1 vote, but require min_score=2.
	votes := map[uint64]*types.Checkpoint{
		0: {Root: block1Root, Slot: 1},
	}

	head := forkchoice.GetForkChoiceHead(store, genesisRoot, votes, 2)
	// Block1 has only 1 vote, below min_score, so head stays at genesis.
	if head != genesisRoot {
		t.Errorf("expected head = genesis (block1 below min score)")
	}
}
