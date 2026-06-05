package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func TestJustifiedCheckpointHandler(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	root := [32]byte{0xab}
	s.SetLatestJustified(&types.Checkpoint{Root: root, Slot: 12})

	rec := httptest.NewRecorder()
	JustifiedCheckpointHandler(s)(rec, httptest.NewRequest(http.MethodGet, "/lean/v0/checkpoints/justified", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	var body checkpointResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Slot != 12 || body.Root != "0xab00000000000000000000000000000000000000000000000000000000000000" {
		t.Fatalf("body=%+v, want slot 12 root 0xab...", body)
	}
}
