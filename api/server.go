package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/node"
	"github.com/geanlabs/gean/types"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// StartAPIServer starts the API server on the given address.
func StartAPIServer(address string, s *node.ConsensusStore) error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /lean/v0/health", handleHealth)
	mux.HandleFunc("GET /lean/v0/states/finalized", handleFinalizedState(s))
	mux.HandleFunc("GET /lean/v0/checkpoints/justified", handleJustifiedCheckpoint(s))
	mux.HandleFunc("GET /lean/v0/fork_choice", handleForkChoice(s))

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("api listen: %w", err)
	}

	logger.Info(logger.Network, "api server listening on %s", address)
	return http.Serve(listener, mux)
}

// StartMetricsServer starts the metrics server on the given address.
func StartMetricsServer(address string) error {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("metrics listen: %w", err)
	}

	logger.Info(logger.Network, "metrics server listening on %s", address)
	return http.Serve(listener, mux)
}

// handleHealth returns a simple health check.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"healthy","service":"gean"}`))
}

// handleFinalizedState returns the finalized state as SSZ bytes.
// Zeros state_root in latest_block_header for canonical post-state form.
func handleFinalizedState(s *node.ConsensusStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		finalized := s.LatestFinalized()
		state := s.GetState(finalized.Root)
		if state == nil {
			http.Error(w, "finalized state not available", http.StatusServiceUnavailable)
			return
		}

		// Zero state_root to match canonical post-state representation.
		
		state.LatestBlockHeader.StateRoot = types.ZeroRoot

		data, err := state.MarshalSSZ()
		if err != nil {
			http.Error(w, "ssz marshal failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(data)
	}
}

// handleJustifiedCheckpoint returns the justified checkpoint as JSON.
func handleJustifiedCheckpoint(s *node.ConsensusStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cp := s.LatestJustified()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"slot": cp.Slot,
			"root": fmt.Sprintf("0x%x", cp.Root),
		})
	}
}

// handleForkChoice returns fork choice info as JSON.
func handleForkChoice(s *node.ConsensusStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		head := s.Head()
		justified := s.LatestJustified()
		finalized := s.LatestFinalized()
		safeTarget := s.SafeTarget()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"head":       fmt.Sprintf("0x%x", head),
			"justified":  map[string]interface{}{"slot": justified.Slot, "root": fmt.Sprintf("0x%x", justified.Root)},
			"finalized":  map[string]interface{}{"slot": finalized.Slot, "root": fmt.Sprintf("0x%x", finalized.Root)},
			"safe_target": fmt.Sprintf("0x%x", safeTarget),
		})
	}
}
