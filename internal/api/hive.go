//go:build hive_testdriver

package api

import (
	"net/http"

	"github.com/geanlabs/gean/internal/api/testdriver"
	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/store"
)

// NewHandlerWithTestDriver serves the node API plus the hive test-driver routes.
func NewHandlerWithTestDriver(s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) *http.ServeMux {
	mux := NewHandler(s, fc, aggCtl)
	testdriver.RegisterRoutes(mux, testdriver.NewSession())
	return mux
}
