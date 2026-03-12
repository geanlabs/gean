package handlers

import (
	"net/http"

	"github.com/geanlabs/gean/chain/forkchoice"
)

// FinalizedState returns a handler for the finalized state endpoint.
func FinalizedState(storeGetter func() *forkchoice.Store) http.HandlerFunc {
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

		sszBytes, ok, err := store.FinalizedStateSSZ()
		if err != nil {
			http.Error(w, "Encoding failed", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "Finalized state not available", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(sszBytes)
	}
}
