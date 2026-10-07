package api

import (
	"net/http"

	"github.com/geanlabs/gean/consensus/forkchoice"
	"github.com/geanlabs/gean/role"
	"github.com/geanlabs/gean/storage/store"
)

// NewHandler serves the node API over the consensus store and the engine's
// published fork choice view.
func NewHandler(s *store.ConsensusStore, forkChoiceView func() *forkchoice.View, aggCtl *role.Controller) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /lean/v0/health", HealthHandler)
	mux.HandleFunc("GET /lean/v0/states/finalized", FinalizedStateHandler(s))
	mux.HandleFunc("GET /lean/v0/blocks/finalized", FinalizedBlockHandler(s))
	mux.HandleFunc("GET /lean/v0/checkpoints/justified", JustifiedCheckpointHandler(s))
	mux.HandleFunc("GET /lean/v0/fork_choice", ForkChoiceHandler(forkChoiceView))
	mux.HandleFunc("GET /lean/v0/admin/aggregator", AggregatorStatusHandler(aggCtl))
	mux.HandleFunc("POST /lean/v0/admin/aggregator", AggregatorToggleHandler(aggCtl))

	return mux
}
