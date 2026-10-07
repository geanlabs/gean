//go:build hive_testdriver

package api

import (
	"github.com/geanlabs/gean/crypto"
	"net/http"

	"github.com/geanlabs/gean/api/testdriver"
	"github.com/geanlabs/gean/consensus/forkchoice"
	"github.com/geanlabs/gean/role"
	"github.com/geanlabs/gean/storage/store"
)

// NewHandlerWithTestDriver serves the node API plus the hive test-driver routes,
// which verify signatures with scheme.
func NewHandlerWithTestDriver(s *store.ConsensusStore, forkChoiceView func() *forkchoice.View, aggCtl *role.Controller, scheme crypto.Scheme) *http.ServeMux {
	mux := NewHandler(s, forkChoiceView, aggCtl)
	testdriver.RegisterRoutes(mux, testdriver.NewSession(scheme))
	return mux
}
