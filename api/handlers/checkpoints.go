package handlers

import (
	"encoding/json"
	"net/http"

	apitypes "github.com/geanlabs/gean/api/types"
	"github.com/geanlabs/gean/chain/forkchoice"
)

// JustifiedCheckpoint returns a handler for the justified checkpoint endpoint.
func JustifiedCheckpoint(storeGetter func() *forkchoice.Store) http.HandlerFunc {
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
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apitypes.CheckpointResponse{
			Slot: snap.Justified.Slot,
			Root: hex32(snap.Justified.Root),
		})
	}
}
