//go:build hive_testdriver

package main

import (
	"net/http"
	"os"

	"github.com/geanlabs/gean/internal/api"
	"github.com/geanlabs/gean/internal/api/testdriver"
	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/store"
)

func apiHandler(s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) http.Handler {
	if testdriver.IsEnabled(os.Getenv(testdriver.EnvVar)) {
		logger.Info(logger.Node, "%s=1: enabling test-driver routes", testdriver.EnvVar)
		return api.NewHandlerWithTestDriver(s, fc, aggCtl)
	}
	return api.NewHandler(s, fc, aggCtl)
}
