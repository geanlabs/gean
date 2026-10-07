//go:build hive_testdriver

package api

import (
	"github.com/geanlabs/gean/crypto/xmss"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geanlabs/gean/db"
	"github.com/geanlabs/gean/role"
	"github.com/geanlabs/gean/store"
)

func TestBuildAPIMuxWithTestDriverRegistersRoutes(t *testing.T) {
	s := store.NewConsensusStore(db.NewInMemoryBackend())
	mux := NewHandlerWithTestDriver(s, nil, role.New(false), xmss.NewScheme())

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
