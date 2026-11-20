package api

import (
	"net/http"

	"github.com/geanlabs/gean/internal/lean/forkchoice"
	"github.com/geanlabs/gean/internal/lean/role"
	"github.com/geanlabs/gean/internal/lean/store"
)

// Handler serves the /lean/v0 API. The caller binds, serves and shuts it
// down with core/httpapi.
func Handler(s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) http.Handler {
	return buildAPIMux(s, fc, aggCtl)
}
