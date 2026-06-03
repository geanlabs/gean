package api

import (
	"fmt"
	"net/http"

	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/store"
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

func ForkChoiceHandler(s *store.ConsensusStore, fc *forkchoice.ForkChoice) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		head := s.Head()
		justified := s.LatestJustified()
		finalized := s.LatestFinalized()
		safeTarget := s.SafeTarget()

		nodes := make([]forkChoiceNode, 0)
		if fc != nil {
			for _, pn := range fc.Nodes() {
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
