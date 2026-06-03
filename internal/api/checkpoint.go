package api

import (
	"fmt"
	"net/http"

	"github.com/geanlabs/gean/internal/store"
)

type checkpointResponse struct {
	Slot uint64 `json:"slot"`
	Root string `json:"root"`
}

func JustifiedCheckpointHandler(s *store.ConsensusStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cp := s.LatestJustified()
		writeJSON(w, http.StatusOK, checkpointResponse{
			Slot: cp.Slot,
			Root: fmt.Sprintf("0x%x", cp.Root),
		})
	}
}
