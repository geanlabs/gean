//go:build hive_testdriver

package main

import (
	"os"

	"github.com/geanlabs/gean/internal/api"
	"github.com/geanlabs/gean/internal/api/testdriver"
	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/store"
)

func startAPIServer(address string, s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) error {
	if testdriver.IsEnabled(os.Getenv(testdriver.EnvVar)) {
		logger.Info(logger.Node, "%s=1: enabling test-driver routes", testdriver.EnvVar)
		return api.StartAPIServerWithTestDriver(address, s, fc, aggCtl)
	}
	return api.StartAPIServer(address, s, fc, aggCtl)
}
