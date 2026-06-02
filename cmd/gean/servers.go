package main

import (
	"github.com/geanlabs/gean/internal/api"
	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/store"
)

func startHTTPServers(cfg config, s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) (string, string) {
	apiAddr := cfg.apiAddress()
	metricsAddr := cfg.metricsAddress()

	go func() {
		if err := startAPIServer(apiAddr, s, fc, aggCtl); err != nil {
			logger.Error(logger.Node, "api server error: %v", err)
		}
	}()

	go func() {
		if err := api.StartMetricsServer(metricsAddr); err != nil {
			logger.Error(logger.Node, "metrics server error: %v", err)
		}
	}()

	return apiAddr, metricsAddr
}
