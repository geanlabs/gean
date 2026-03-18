package handlers

import (
	"encoding/json"
	"net/http"

	apitypes "github.com/geanlabs/gean/api/types"
)

const (
	healthStatus  = "healthy"
	healthService = "lean-rpc-api"
)

// Health returns a handler for the health endpoint.
func Health() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apitypes.HealthResponse{
			Status:  healthStatus,
			Service: healthService,
		})
	}
}
