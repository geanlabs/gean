package unit

import (
	"testing"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/storage/memory"
	"github.com/geanlabs/gean/types"
)

func makeBlock(slot, proposer uint64, parent [32]byte) *types.Block {
	return &types.Block{
		Slot:          slot,
		ProposerIndex: proposer,
		ParentRoot:    parent,
		StateRoot:     types.ZeroHash,
		Body:          &types.BlockBody{Attestations: []*types.Attestation{}},
	}
}

// makeGhostAttestation creates a minimal attestation for GHOST tests.
func makeGhostAttestation(validatorID uint64, headRoot [32]byte, headSlot uint64) *types.Attestation {
	cp := &types.Checkpoint{Root: headRoot, Slot: headSlot}
	return &types.Attestation{
		ValidatorID: validatorID,
		Data: &types.AttestationData{
			Slot:   headSlot,
			Head:   cp,
			Target: cp,
			Source: cp,
		},
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
	atts := map[uint64]*types.Attestation{
		0: makeGhostAttestation(0, block2Root, 2),
	}

	head := forkchoice.GetForkChoiceHead(store, genesisRoot, atts, 0)
	if head != block2Root {
		t.Errorf("expected head = block2, got %x", head[:4])
	}
}

func TestGetForkChoiceHeadNoVotes(t *testing.T) {
	store := memory.New()

	genesis := makeBlock(0, 0, types.ZeroHash)
	genesisRoot, _ := genesis.HashTreeRoot()
	store.PutBlock(genesisRoot, genesis)

	atts := map[uint64]*types.Attestation{}

	head := forkchoice.GetForkChoiceHead(store, genesisRoot, atts, 0)
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
	atts := map[uint64]*types.Attestation{
		0: makeGhostAttestation(0, blockARoot, 1),
		1: makeGhostAttestation(1, blockARoot, 1),
		2: makeGhostAttestation(2, blockBRoot, 1),
	}

	head := forkchoice.GetForkChoiceHead(store, genesisRoot, atts, 0)
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
	atts := map[uint64]*types.Attestation{
		0: makeGhostAttestation(0, block1Root, 1),
	}

	head := forkchoice.GetForkChoiceHead(store, genesisRoot, atts, 2)
	// Block1 has only 1 vote, below min_score, so head stays at genesis.
	if head != genesisRoot {
		t.Errorf("expected head = genesis (block1 below min score)")
	}
}
