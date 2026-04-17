package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"

	"github.com/geanlabs/gean/forkchoice"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/node"
	"github.com/geanlabs/gean/types"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// StartAPIServer starts the API server on the given address.
func StartAPIServer(address string, s *node.ConsensusStore, fc *forkchoice.ForkChoice) error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /lean/v0/health", HealthHandler)
	mux.HandleFunc("GET /lean/v0/states/finalized", FinalizedStateHandler(s))
	mux.HandleFunc("GET /lean/v0/checkpoints/justified", JustifiedCheckpointHandler(s))
	mux.HandleFunc("GET /lean/v0/fork_choice", ForkChoiceHandler(s, fc))

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("api listen: %w", err)
	}

	logger.Info(logger.Network, "api server listening on %s", address)
	return http.Serve(listener, mux)
}

// StartMetricsServer starts the metrics server on the given address.
// Also exposes Go runtime pprof endpoints under /debug/pprof/ on the same
// port for heap, goroutine, CPU, block, and mutex profiling.
func StartMetricsServer(address string) error {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("metrics listen: %w", err)
	}

	logger.Info(logger.Network, "metrics server listening on %s", address)
	return http.Serve(listener, mux)
}

// handleHealth returns a simple health check.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"healthy","service":"lean-rpc-api"}`))
}

// handleFinalizedState returns the finalized state as SSZ bytes.
// Zeros state_root in latest_block_header for canonical post-state form.
func FinalizedStateHandler(s *node.ConsensusStore) http.HandlerFunc {
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
func JustifiedCheckpointHandler(s *node.ConsensusStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cp := s.LatestJustified()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"slot": cp.Slot,
			"root": fmt.Sprintf("0x%x", cp.Root),
		})
	}
}

// handleForkChoice returns fork choice info as JSON, matching leanSpec's
// api_endpoint fixture schema: {nodes[], head, justified, finalized, safe_target, validator_count}.
func ForkChoiceHandler(s *node.ConsensusStore, fc *forkchoice.ForkChoice) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		head := s.Head()
		justified := s.LatestJustified()
		finalized := s.LatestFinalized()
		safeTarget := s.SafeTarget()

		// Build nodes[] array from proto-array snapshot. proposer_index comes
		// from the block header (proto-array doesn't track it).
		nodes := make([]map[string]interface{}, 0)
		if fc != nil && fc.Array != nil {
			for _, pn := range fc.Array.Nodes() {
				var proposerIndex uint64
				if hdr := s.GetBlockHeader(pn.Root); hdr != nil {
					proposerIndex = hdr.ProposerIndex
				}
				nodes = append(nodes, map[string]interface{}{
					"root":           fmt.Sprintf("0x%x", pn.Root),
					"slot":           pn.Slot,
					"parent_root":    fmt.Sprintf("0x%x", pn.ParentRoot),
					"proposer_index": proposerIndex,
					"weight":         pn.Weight,
				})
			}
		}

		// validator_count from the head state (fallback to 0 if unavailable).
		var validatorCount uint64
		if headState := s.GetState(head); headState != nil {
			validatorCount = headState.NumValidators()
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"nodes":           nodes,
			"head":            fmt.Sprintf("0x%x", head),
			"justified":       map[string]interface{}{"slot": justified.Slot, "root": fmt.Sprintf("0x%x", justified.Root)},
			"finalized":       map[string]interface{}{"slot": finalized.Slot, "root": fmt.Sprintf("0x%x", finalized.Root)},
			"safe_target":     fmt.Sprintf("0x%x", safeTarget),
			"validator_count": validatorCount,
		})
	}
}
