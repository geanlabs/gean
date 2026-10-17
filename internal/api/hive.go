//go:build hive_testdriver

package api

import (
	"net/http"

	"github.com/geanlabs/gean/internal/api/testdriver"
	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/store"
)

// NewAPIServerWithTestDriver returns the API server with the hive test-driver
// routes added.
func NewAPIServerWithTestDriver(s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) *http.Server {
	return &http.Server{Handler: buildAPIMuxWithTestDriver(s, fc, aggCtl)}
}

func buildAPIMuxWithTestDriver(s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) *http.ServeMux {
	mux := buildAPIMux(s, fc, aggCtl)
	testdriver.RegisterRoutes(mux, testdriver.NewSession())
	return mux
}
