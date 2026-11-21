package api

import (
	"net/http"

	"github.com/geanlabs/gean/internal/lean/api/testdriver"
)

// TestDriverHandler serves the hive test-driver routes and the health check,
// and nothing else, for a node started only to replay leanSpec fixtures.
func TestDriverHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /lean/v0/health", HealthHandler)
	testdriver.RegisterRoutes(mux, testdriver.NewSession())
	return mux
}
