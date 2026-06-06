package api

import (
	"net/http"

	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/store"
)

func buildAPIMux(s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /lean/v0/health", HealthHandler)
	mux.HandleFunc("GET /lean/v0/states/finalized", FinalizedStateHandler(s))
	mux.HandleFunc("GET /lean/v0/blocks/finalized", FinalizedBlockHandler(s))
	mux.HandleFunc("GET /lean/v0/checkpoints/justified", JustifiedCheckpointHandler(s))
	mux.HandleFunc("GET /lean/v0/fork_choice", ForkChoiceHandler(s, fc))
	mux.HandleFunc("GET /lean/v0/admin/aggregator", AggregatorStatusHandler(aggCtl))
	mux.HandleFunc("POST /lean/v0/admin/aggregator", AggregatorToggleHandler(aggCtl))

	return mux
}
