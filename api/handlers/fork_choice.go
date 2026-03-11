package handlers

import (
	"encoding/json"
	"net/http"

	apitypes "github.com/geanlabs/gean/api/types"
	"github.com/geanlabs/gean/chain/forkchoice"
)

// ForkChoice returns a handler for the fork choice endpoint.
func ForkChoice(storeGetter func() *forkchoice.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		store := storeGetter()
		if store == nil {
			http.Error(w, "Store not initialized", http.StatusServiceUnavailable)
			return
		}

		snap := store.ForkChoiceSnapshot()
		nodes := make([]apitypes.ForkChoiceNode, 0, len(snap.Nodes))
		for _, n := range snap.Nodes {
			nodes = append(nodes, apitypes.ForkChoiceNode{
				Root:          hex32(n.Root),
				Slot:          n.Slot,
				ParentRoot:    hex32(n.ParentRoot),
				ProposerIndex: n.ProposerIndex,
				Weight:        n.Weight,
			})
		}

		resp := apitypes.ForkChoiceResponse{
			Nodes: nodes,
			Head:  hex32(snap.Head),
			Justified: apitypes.CheckpointResponse{
				Slot: snap.Justified.Slot,
				Root: hex32(snap.Justified.Root),
			},
			Finalized: apitypes.CheckpointResponse{
				Slot: snap.Finalized.Slot,
				Root: hex32(snap.Finalized.Root),
			},
			SafeTarget:     hex32(snap.SafeTarget),
			ValidatorCount: snap.ValidatorCount,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}
