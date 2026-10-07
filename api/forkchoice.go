package api

import (
	"fmt"
	"net/http"

	"github.com/geanlabs/gean/forkchoice"
	"github.com/geanlabs/gean/store"
)

type forkChoiceResponse struct {
	Nodes          []forkChoiceNode   `json:"nodes"`
	Head           string             `json:"head"`
	Justified      checkpointResponse `json:"justified"`
	Finalized      checkpointResponse `json:"finalized"`
	SafeTarget     string             `json:"safe_target"`
	ValidatorCount uint64             `json:"validator_count"`
}

type forkChoiceNode struct {
	Root          string `json:"root"`
	Slot          uint64 `json:"slot"`
	ParentRoot    string `json:"parent_root"`
	ProposerIndex uint64 `json:"proposer_index"`
	Weight        int64  `json:"weight"`
}

// ForkChoiceHandler serves the latest fork choice view. Everything else it
// reports is read from the store by root, which never changes once written, so
// the response is consistent with the view.
func ForkChoiceHandler(s *store.ConsensusStore, view func() *forkchoice.View) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v := view()
		if v == nil {
			http.Error(w, "fork choice not available", http.StatusServiceUnavailable)
			return
		}
		head, justified, finalized, safeTarget := v.Head, v.Justified, v.Finalized, v.SafeTarget

		nodes := make([]forkChoiceNode, 0, len(v.Nodes))
		for _, pn := range v.Nodes {
			var proposerIndex uint64
			if hdr := s.GetBlockHeader(pn.Root); hdr != nil {
				proposerIndex = hdr.ProposerIndex
			}
			nodes = append(nodes, forkChoiceNode{
				Root:          fmt.Sprintf("0x%x", pn.Root),
				Slot:          pn.Slot,
				ParentRoot:    fmt.Sprintf("0x%x", pn.ParentRoot),
				ProposerIndex: proposerIndex,
				Weight:        pn.Weight,
			})
		}

		var validatorCount uint64
		if headState := s.GetState(head); headState != nil {
			validatorCount = headState.NumValidators()
		}

		writeJSON(w, http.StatusOK, forkChoiceResponse{
			Nodes: nodes,
			Head:  fmt.Sprintf("0x%x", head),
			Justified: checkpointResponse{
				Slot: justified.Slot,
				Root: fmt.Sprintf("0x%x", justified.Root),
			},
			Finalized: checkpointResponse{
				Slot: finalized.Slot,
				Root: fmt.Sprintf("0x%x", finalized.Root),
			},
			SafeTarget:     fmt.Sprintf("0x%x", safeTarget),
			ValidatorCount: validatorCount,
		})
	}
}
