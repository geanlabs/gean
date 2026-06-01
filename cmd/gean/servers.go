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

func startHTTPServers(cfg config, s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) (string, string) {
	apiAddr := cfg.apiAddress()
	metricsAddr := cfg.metricsAddress()

	go func() {
		var apiErr error
		if testdriver.IsEnabled(os.Getenv(testdriver.EnvVar)) {
			logger.Info(logger.Node, "%s=1: enabling test-driver routes", testdriver.EnvVar)
			apiErr = api.StartAPIServerWithTestDriver(apiAddr, s, fc, aggCtl)
		} else {
			apiErr = api.StartAPIServer(apiAddr, s, fc, aggCtl)
		}
		if apiErr != nil {
			logger.Error(logger.Node, "api server error: %v", apiErr)
		}
	}()

	go func() {
		if err := api.StartMetricsServer(metricsAddr); err != nil {
			logger.Error(logger.Node, "metrics server error: %v", err)
		}
	}()

	return apiAddr, metricsAddr
}
