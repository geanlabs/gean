package api

import (
	"fmt"
	"net/http"

	"github.com/geanlabs/gean/consensus/forkchoice"
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

// ForkChoiceHandler serves the latest published fork choice view. It reads
// nothing else, so the response is consistent even while pruning deletes the
// blocks and states the view refers to.
func ForkChoiceHandler(view func() *forkchoice.View) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v := view()
		if v == nil {
			http.Error(w, "fork choice not available", http.StatusServiceUnavailable)
			return
		}

		nodes := make([]forkChoiceNode, 0, len(v.Nodes))
		for _, n := range v.Nodes {
			nodes = append(nodes, forkChoiceNode{
				Root:          fmt.Sprintf("0x%x", n.Root),
				Slot:          n.Slot,
				ParentRoot:    fmt.Sprintf("0x%x", n.ParentRoot),
				ProposerIndex: n.ProposerIndex,
				Weight:        n.Weight,
			})
		}

		writeJSON(w, http.StatusOK, forkChoiceResponse{
			Nodes: nodes,
			Head:  fmt.Sprintf("0x%x", v.Head),
			Justified: checkpointResponse{
				Slot: v.Justified.Slot,
				Root: fmt.Sprintf("0x%x", v.Justified.Root),
			},
			Finalized: checkpointResponse{
				Slot: v.Finalized.Slot,
				Root: fmt.Sprintf("0x%x", v.Finalized.Root),
			},
			SafeTarget:     fmt.Sprintf("0x%x", v.SafeTarget),
			ValidatorCount: v.ValidatorCount,
		})
	}
}
