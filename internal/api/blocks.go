package api

import (
	"net/http"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func FinalizedBlockHandler(s *store.ConsensusStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		finalized := s.LatestFinalized()
		if finalized.Root == types.ZeroRoot {
			http.Error(w, "no finalized block yet", http.StatusNotFound)
			return
		}
		signedBlock := s.GetSignedBlock(finalized.Root)
		if signedBlock == nil {
			http.Error(w, "finalized block not available", http.StatusNotFound)
			return
		}
		data, err := signedBlock.MarshalSSZ()
		if err != nil {
			http.Error(w, "ssz marshal failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(data)
	}
}
