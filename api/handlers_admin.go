package api

import (
	"encoding/json"
	"net/http"

	"github.com/geanlabs/gean/node"
)

// AggregatorStatusHandler serves GET /lean/v0/admin/aggregator.
// Returns the current aggregator role as {"is_aggregator": bool}.
// Spec: leanSpec PR #636.
func AggregatorStatusHandler(ctl *node.AggregatorController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"is_aggregator": ctl.Get(),
		})
	}
}

// AggregatorToggleHandler serves POST /lean/v0/admin/aggregator.
// Body: {"enabled": bool}. Response: {"is_aggregator": new, "previous": old}.
// Spec: leanSpec PR #636.
//
// 400 conditions (match the spec PR):
//   - empty body
//   - malformed JSON
//   - missing "enabled" field
//   - "enabled" not a boolean
func AggregatorToggleHandler(ctl *node.AggregatorController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode into a pointer so we can distinguish "missing" from "false".
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}
		if body.Enabled == nil {
			http.Error(w, `missing required field "enabled"`, http.StatusBadRequest)
			return
		}

		prev := ctl.Set(*body.Enabled)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"is_aggregator": ctl.Get(),
			"previous":      prev,
		})
	}
}
