package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
)

func TestBuildAPIMuxRegistersRuntimeRoutes(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	mux := buildAPIMux(s, nil, role.New(false))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/lean/v0/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health status=%d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/lean/v0/admin/aggregator", strings.NewReader(`{"enabled":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("aggregator status=%d, want 200", rec.Code)
	}
	var body aggregatorToggleResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode aggregator body: %v", err)
	}
	if !body.IsAggregator {
		t.Fatal("aggregator route did not enable role")
	}
}
