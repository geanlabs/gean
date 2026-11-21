//go:build spectests

package lean

import (
	"testing"

	"github.com/geanlabs/gean/internal/core/storage"
	"github.com/geanlabs/gean/internal/lean/importer"
	"github.com/geanlabs/gean/internal/lean/specfixtures"
	"github.com/geanlabs/gean/internal/lean/store"
	"github.com/geanlabs/gean/internal/lean/types"
)

// runVerifySignatures imports a signed block on top of an anchor state, with
// full signature verification.
func runVerifySignatures(t *testing.T, fixture *specfixtures.VerifySignaturesFixture) {
	anchorState, err := fixture.AnchorState.ToState()
	if err != nil {
		t.Fatalf("anchor state: %v", err)
	}
	s, _ := newAnchoredStore(t, anchorState)

	signedBlock, err := fixture.SignedBlock.ToSignedBlock()
	if err != nil {
		t.Fatalf("signed block: %v", err)
	}
	// The clock sits at the block's slot, as on a live node importing an on-time
	// block; at the default zero it would reject the block as from the future.
	s.SetTime(signedBlock.Block.Slot * types.IntervalsPerSlot)

	rejectionReason := ""
	if fixture.RejectionReason != nil {
		rejectionReason = *fixture.RejectionReason
	}
	checkRejection(t, importer.OnBlock(s, signedBlock), rejectionReason)
}

// newAnchoredStore starts a store from state, as a node does at startup: a
// genesis header's zero state root is filled in before the anchor block root
// is taken from the header.
func newAnchoredStore(t *testing.T, state *types.State) (*store.ConsensusStore, [32]byte) {
	t.Helper()
	header := state.LatestBlockHeader
	if header.StateRoot == types.ZeroRoot {
		stateRoot, err := state.HashTreeRoot()
		if err != nil {
			t.Fatalf("anchor state root: %v", err)
		}
		header.StateRoot = stateRoot
	}
	anchorRoot, err := header.HashTreeRoot()
	if err != nil {
		t.Fatalf("anchor block root: %v", err)
	}
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	if err := s.PutAnchor(anchorRoot, header, state); err != nil {
		t.Fatalf("put anchor: %v", err)
	}
	return s, anchorRoot
}
