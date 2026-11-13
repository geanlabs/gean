//go:build !hive_testdriver

package node

import (
	"net/http"

	"github.com/geanlabs/gean/lean/consensus/forkchoice"
	"github.com/geanlabs/gean/lean/internal/aggregation/role"
	"github.com/geanlabs/gean/lean/internal/api"
	"github.com/geanlabs/gean/lean/internal/store"
)

func newAPIServer(s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) *http.Server {
	return api.NewAPIServer(s, fc, aggCtl)
}
