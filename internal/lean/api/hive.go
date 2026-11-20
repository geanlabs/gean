//go:build hive_testdriver

package api

import (
	"net/http"

	"github.com/geanlabs/gean/internal/lean/api/testdriver"
	"github.com/geanlabs/gean/internal/lean/forkchoice"
	"github.com/geanlabs/gean/internal/lean/role"
	"github.com/geanlabs/gean/internal/lean/store"
)

// HandlerWithTestDriver serves the /lean/v0 API with the hive test-driver
// routes added.
func HandlerWithTestDriver(s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) http.Handler {
	return buildAPIMuxWithTestDriver(s, fc, aggCtl)
}

func buildAPIMuxWithTestDriver(s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) *http.ServeMux {
	mux := buildAPIMux(s, fc, aggCtl)
	testdriver.RegisterRoutes(mux, testdriver.NewSession())
	return mux
}
