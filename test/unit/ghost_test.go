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

// makeGhostAttestation creates a minimal signed attestation for GHOST tests.
func makeGhostAttestation(validatorID uint64, headRoot [32]byte, headSlot uint64) *types.SignedAttestation {
	cp := &types.Checkpoint{Root: headRoot, Slot: headSlot}
	return &types.SignedAttestation{
		Message: &types.Attestation{
			ValidatorID: validatorID,
			Data: &types.AttestationData{
				Slot:   headSlot,
				Head:   cp,
				Target: cp,
				Source: cp,
			},
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
	atts := map[uint64]*types.SignedAttestation{
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

	atts := map[uint64]*types.SignedAttestation{}

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
	atts := map[uint64]*types.SignedAttestation{
		0: makeGhostAttestation(0, blockARoot, 1),
		1: makeGhostAttestation(1, blockARoot, 1),
		2: makeGhostAttestation(2, blockBRoot, 1),
	}

	head := forkchoice.GetForkChoiceHead(store, genesisRoot, atts, 0)
	if head != blockARoot {
		t.Errorf("expected head = blockA (more votes)")
	}
}

// TestGetForkChoiceHeadTiebreakBySlotThenHash verifies that with equal vote
// weight, higher slot wins first, and hash only breaks ties when slot is equal.
func TestGetForkChoiceHeadTiebreakBySlotThenHash(t *testing.T) {
	store := memory.New()

	genesis := makeBlock(0, 0, types.ZeroHash)
	genesisRoot, _ := genesis.HashTreeRoot()
	store.PutBlock(genesisRoot, genesis)

	// Fork A at slot 3 (higher slot).
	blockA := makeBlock(3, 0, genesisRoot)
	blockARoot, _ := blockA.HashTreeRoot()
	store.PutBlock(blockARoot, blockA)

	// Fork B at slot 1 (lower slot).
	blockB := makeBlock(1, 1, genesisRoot)
	blockBRoot, _ := blockB.HashTreeRoot()
	store.PutBlock(blockBRoot, blockB)

	// Equal weight: 1 vote each.
	atts := map[uint64]*types.SignedAttestation{
		0: makeGhostAttestation(0, blockARoot, 3),
		1: makeGhostAttestation(1, blockBRoot, 1),
	}

	head := forkchoice.GetForkChoiceHead(store, genesisRoot, atts, 0)
	if head != blockARoot {
		t.Errorf("expected higher-slot blockA to win, got %x", head[:4])
	}
}

func TestGetForkChoiceHeadTiebreakByHashWhenSlotsEqual(t *testing.T) {
	store := memory.New()

	genesis := makeBlock(0, 0, types.ZeroHash)
	genesisRoot, _ := genesis.HashTreeRoot()
	store.PutBlock(genesisRoot, genesis)

	// Same slot, equal weight; hash decides.
	blockA := makeBlock(2, 0, genesisRoot)
	blockARoot, _ := blockA.HashTreeRoot()
	store.PutBlock(blockARoot, blockA)

	blockB := makeBlock(2, 1, genesisRoot)
	blockBRoot, _ := blockB.HashTreeRoot()
	store.PutBlock(blockBRoot, blockB)

	atts := map[uint64]*types.SignedAttestation{
		0: makeGhostAttestation(0, blockARoot, 2),
		1: makeGhostAttestation(1, blockBRoot, 2),
	}

	head := forkchoice.GetForkChoiceHead(store, genesisRoot, atts, 0)
	expected := blockARoot
	if hashGreaterTest(blockBRoot, blockARoot) {
		expected = blockBRoot
	}
	if head != expected {
		t.Errorf("expected larger-hash block %x, got %x", expected[:4], head[:4])
	}
}

func hashGreaterTest(a, b [32]byte) bool {
	for i := 0; i < 32; i++ {
		if a[i] > b[i] {
			return true
		}
		if a[i] < b[i] {
			return false
		}
	}
	return false
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
	atts := map[uint64]*types.SignedAttestation{
		0: makeGhostAttestation(0, block1Root, 1),
	}

	head := forkchoice.GetForkChoiceHead(store, genesisRoot, atts, 2)
	// Block1 has only 1 vote, below min_score, so head stays at genesis.
	if head != genesisRoot {
		t.Errorf("expected head = genesis (block1 below min score)")
	}
}
