//go:build hive_testdriver

package node

import (
	"net/http"
	"os"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/lean/consensus/forkchoice"
	"github.com/geanlabs/gean/lean/internal/aggregation/role"
	"github.com/geanlabs/gean/lean/internal/api"
	"github.com/geanlabs/gean/lean/internal/api/testdriver"
	"github.com/geanlabs/gean/lean/internal/store"
)

func newAPIServer(s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) *http.Server {
	if testdriver.IsEnabled(os.Getenv(testdriver.EnvVar)) {
		logger.Info(logger.Node, "%s=1: enabling test-driver routes", testdriver.EnvVar)
		return api.NewAPIServerWithTestDriver(s, fc, aggCtl)
	}
	return api.NewAPIServer(s, fc, aggCtl)
}
