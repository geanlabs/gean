package api

import (
	"net/http"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func FinalizedStateHandler(s *store.ConsensusStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		finalized := s.LatestFinalized()
		state := s.GetState(finalized.Root)
		if state == nil {
			http.Error(w, "finalized state not available", http.StatusServiceUnavailable)
			return
		}
		if state.LatestBlockHeader == nil {
			http.Error(w, "finalized state header not available", http.StatusServiceUnavailable)
			return
		}

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
