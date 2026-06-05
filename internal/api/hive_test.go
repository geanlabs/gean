//go:build hive_testdriver

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
)

func TestBuildAPIMuxWithTestDriverRegistersRoutes(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	mux := buildAPIMuxWithTestDriver(s, nil, role.New(false))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/lean/v0/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health status=%d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/lean/v0/test_driver/state_transition/run", strings.NewReader("not json"))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("testdriver status=%d, want 400", rec.Code)
	}
}
