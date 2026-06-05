package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func TestFinalizedStateHandler_ReturnsCanonicalSSZ(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())

	var root [32]byte
	root[0] = 1
	state := testState(7)
	state.LatestBlockHeader.StateRoot = [32]byte{0xaa}
	s.InsertState(root, state)
	s.SetLatestFinalized(&types.Checkpoint{Root: root, Slot: 7})

	rec := httptest.NewRecorder()
	FinalizedStateHandler(s)(rec, httptest.NewRequest(http.MethodGet, "/lean/v0/states/finalized", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("Content-Type=%q, want application/octet-stream", got)
	}

	var got types.State
	if err := got.UnmarshalSSZ(rec.Body.Bytes()); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if got.Slot != state.Slot {
		t.Fatalf("slot=%d, want %d", got.Slot, state.Slot)
	}
	if got.LatestBlockHeader.StateRoot != types.ZeroRoot {
		t.Fatalf("state_root=0x%x, want zero", got.LatestBlockHeader.StateRoot)
	}
}

func TestFinalizedStateHandler_UnavailableWhenMissing(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	s.SetLatestFinalized(&types.Checkpoint{Root: [32]byte{1}, Slot: 7})

	rec := httptest.NewRecorder()
	FinalizedStateHandler(s)(rec, httptest.NewRequest(http.MethodGet, "/lean/v0/states/finalized", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503", rec.Code)
	}
}

func testState(slot uint64) *types.State {
	return &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     slot,
		LatestBlockHeader:        &types.BlockHeader{Slot: slot},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
}
