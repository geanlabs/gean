package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geanlabs/gean/node"
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

// TestFinalizedStateHandlerAlignsCheckpoints reproduces the scenario where the
// stored post-state's embedded checkpoints lag the store's live view: the state
// at the finalized block root was computed during that block's own STF, so its
// LatestFinalized reflects what block-R's own attestations finalized (an
// ancestor), not the fact that R itself has since become finalized.
//
// The handler must project the store's current finalized checkpoint onto the
// served blob so /lean/v0/states/finalized stays consistent with
// /lean/v0/fork_choice. It must also preserve the
// latest_justified.slot >= latest_finalized.slot invariant.
func TestFinalizedStateHandlerAlignsCheckpoints(t *testing.T) {
	s := node.NewConsensusStore(storage.NewInMemoryBackend())

	// Block R at slot 11 — the block the store now considers finalized.
	var finalizedRoot [32]byte
	finalizedRoot[0] = 0xaa

	header := &types.BlockHeader{
		Slot:       11,
		ParentRoot: [32]byte{0x01},
		StateRoot:  [32]byte{0x02},
	}

	// Post-state at R: its own attestations finalized an ancestor at slot 8
	// and justified one at slot 9, which is the lagging view we expect to see
	// overwritten at serve time.
	state := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     11,
		LatestBlockHeader:        header,
		LatestJustified:          &types.Checkpoint{Root: [32]byte{0x03}, Slot: 9},
		LatestFinalized:          &types.Checkpoint{Root: [32]byte{0x04}, Slot: 8},
		HistoricalBlockHashes:    [][]byte{make([]byte, 32)},
		JustifiedSlots:           types.NewBitlistSSZ(16),
		Validators:               []*types.Validator{{AttestationPubkey: [52]byte{1}, Index: 0}},
		JustificationsRoots:      [][]byte{make([]byte, 32)},
		JustificationsValidators: types.NewBitlistSSZ(8),
	}

	s.InsertState(finalizedRoot, state)

	// Store's live view: R is finalized at slot 11, and a later block has
	// already justified slot 12.
	storeFinalized := &types.Checkpoint{Root: finalizedRoot, Slot: 11}
	storeJustified := &types.Checkpoint{Root: [32]byte{0x05}, Slot: 12}
	s.SetLatestFinalized(storeFinalized)
	s.SetLatestJustified(storeJustified)

	req := httptest.NewRequest(http.MethodGet, "/lean/v0/states/finalized", nil)
	rec := httptest.NewRecorder()
	FinalizedStateHandler(s)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	served := &types.State{}
	if err := served.UnmarshalSSZ(body); err != nil {
		t.Fatalf("unmarshal served state: %v", err)
	}

	if served.LatestFinalized.Slot != storeFinalized.Slot {
		t.Fatalf("latest_finalized.slot: want %d (store), got %d (stale state)",
			storeFinalized.Slot, served.LatestFinalized.Slot)
	}
	if served.LatestFinalized.Root != storeFinalized.Root {
		t.Fatalf("latest_finalized.root mismatch with store")
	}

	if served.LatestJustified.Slot < served.LatestFinalized.Slot {
		t.Fatalf("invariant violated: latest_justified.slot (%d) < latest_finalized.slot (%d)",
			served.LatestJustified.Slot, served.LatestFinalized.Slot)
	}
	if served.LatestJustified.Slot != storeJustified.Slot {
		t.Fatalf("latest_justified.slot: want %d (lifted from store), got %d",
			storeJustified.Slot, served.LatestJustified.Slot)
	}

	// Canonical post-state form: state_root in latest_block_header is zeroed.
	if served.LatestBlockHeader.StateRoot != types.ZeroRoot {
		t.Fatalf("latest_block_header.state_root should be zeroed, got %x",
			served.LatestBlockHeader.StateRoot)
	}
}

// TestFinalizedStateHandlerJustifiedNotLoweredWhenAhead confirms that when the
// state's embedded latest_justified is already at or ahead of the store's
// finalized slot, we do not overwrite it — the invariant is already satisfied
// and the embedded value is the correct one for block-R's era.
func TestFinalizedStateHandlerJustifiedNotLoweredWhenAhead(t *testing.T) {
	s := node.NewConsensusStore(storage.NewInMemoryBackend())

	var finalizedRoot [32]byte
	finalizedRoot[0] = 0xbb

	embeddedJustified := &types.Checkpoint{Root: [32]byte{0x10}, Slot: 20}

	state := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     15,
		LatestBlockHeader:        &types.BlockHeader{Slot: 15},
		LatestJustified:          embeddedJustified,
		LatestFinalized:          &types.Checkpoint{Root: [32]byte{0x11}, Slot: 10},
		HistoricalBlockHashes:    [][]byte{make([]byte, 32)},
		JustifiedSlots:           types.NewBitlistSSZ(16),
		Validators:               []*types.Validator{{AttestationPubkey: [52]byte{1}, Index: 0}},
		JustificationsRoots:      [][]byte{make([]byte, 32)},
		JustificationsValidators: types.NewBitlistSSZ(8),
	}

	s.InsertState(finalizedRoot, state)
	s.SetLatestFinalized(&types.Checkpoint{Root: finalizedRoot, Slot: 15})
	// Store's justified is ahead of embedded, but embedded is already ahead of
	// the new finalized slot, so the handler should leave embedded alone.
	s.SetLatestJustified(&types.Checkpoint{Root: [32]byte{0x12}, Slot: 25})

	req := httptest.NewRequest(http.MethodGet, "/lean/v0/states/finalized", nil)
	rec := httptest.NewRecorder()
	FinalizedStateHandler(s)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}

	served := &types.State{}
	if err := served.UnmarshalSSZ(rec.Body.Bytes()); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if served.LatestJustified.Slot != embeddedJustified.Slot ||
		served.LatestJustified.Root != embeddedJustified.Root {
		t.Fatalf("latest_justified should be preserved from state when already ahead: want slot=%d root=%x, got slot=%d root=%x",
			embeddedJustified.Slot, embeddedJustified.Root,
			served.LatestJustified.Slot, served.LatestJustified.Root)
	}
}

// TestFinalizedStateHandlerMissingStateReturns503 ensures the handler fails
// cleanly when the store has a finalized checkpoint but no corresponding state.
func TestFinalizedStateHandlerMissingStateReturns503(t *testing.T) {
	s := node.NewConsensusStore(storage.NewInMemoryBackend())
	var root [32]byte
	root[0] = 0xcc
	s.SetLatestFinalized(&types.Checkpoint{Root: root, Slot: 5})

	req := httptest.NewRequest(http.MethodGet, "/lean/v0/states/finalized", nil)
	rec := httptest.NewRecorder()
	FinalizedStateHandler(s)(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: want 503, got %d", rec.Code)
	}
}
