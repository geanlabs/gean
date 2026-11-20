package api

import (
	"encoding/json"
	"net/http"

	"github.com/geanlabs/gean/internal/core/httpapi"
	"github.com/geanlabs/gean/internal/lean/role"
)

type aggregatorStatusResponse struct {
	IsAggregator bool `json:"is_aggregator"`
}

type aggregatorToggleRequest struct {
	Enabled *bool `json:"enabled"`
}

type aggregatorToggleResponse struct {
	IsAggregator bool `json:"is_aggregator"`
	Previous     bool `json:"previous"`
}

func AggregatorStatusHandler(ctl *role.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpapi.WriteJSON(w, http.StatusOK, aggregatorStatusResponse{IsAggregator: ctl.Get()})
	}
}

func AggregatorToggleHandler(ctl *role.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body aggregatorToggleRequest
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
		httpapi.WriteJSON(w, http.StatusOK, aggregatorToggleResponse{
			IsAggregator: ctl.Get(),
			Previous:     prev,
		})
	}
}
