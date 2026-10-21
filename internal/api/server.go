package api

import (
	"net/http"

	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/store"
)

// NewAPIServer returns the API server. The caller binds it, serves it and
// shuts it down.
func NewAPIServer(s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) *http.Server {
	return &http.Server{Handler: buildAPIMux(s, fc, aggCtl)}
}
