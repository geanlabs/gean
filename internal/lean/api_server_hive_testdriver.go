//go:build hive_testdriver

package lean

import (
	"net/http"
	"os"

	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/lean/api"
	"github.com/geanlabs/gean/internal/lean/api/testdriver"
	"github.com/geanlabs/gean/internal/lean/forkchoice"
	"github.com/geanlabs/gean/internal/lean/role"
	"github.com/geanlabs/gean/internal/lean/store"
)

func newAPIHandler(s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) http.Handler {
	if testdriver.IsEnabled(os.Getenv(testdriver.EnvVar)) {
		logger.Info(logger.Node, "%s=1: enabling test-driver routes", testdriver.EnvVar)
		return api.HandlerWithTestDriver(s, fc, aggCtl)
	}
	return api.Handler(s, fc, aggCtl)
}
