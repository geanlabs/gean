//go:build hive_testdriver

package api

import (
	"net/http"

	"github.com/geanlabs/gean/api/testdriver"
	"github.com/geanlabs/gean/forkchoice"
	"github.com/geanlabs/gean/role"
	"github.com/geanlabs/gean/store"
)

// NewHandlerWithTestDriver serves the node API plus the hive test-driver routes.
func NewHandlerWithTestDriver(s *store.ConsensusStore, forkChoiceView func() *forkchoice.View, aggCtl *role.Controller) *http.ServeMux {
	mux := NewHandler(s, forkChoiceView, aggCtl)
	testdriver.RegisterRoutes(mux, testdriver.NewSession())
	return mux
}
