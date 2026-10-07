package main

import (
	"context"

	"github.com/geanlabs/gean/api"
	"github.com/geanlabs/gean/forkchoice"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/role"
	"github.com/geanlabs/gean/store"
	"github.com/geanlabs/gean/tasks"
)

func startHTTPServers(ctx context.Context, services *tasks.Group, cfg config, s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) (string, string) {
	apiAddr := cfg.apiAddress()
	metricsAddr := cfg.metricsAddress()

	services.Go(func() {
		if err := api.Serve(ctx, "api", apiAddr, apiHandler(s, fc, aggCtl)); err != nil {
			logger.Error(logger.Node, "api server error: %v", err)
		}
	})

	services.Go(func() {
		if err := api.Serve(ctx, "metrics", metricsAddr, api.NewMetricsHandler()); err != nil {
			logger.Error(logger.Node, "metrics server error: %v", err)
		}
	})

	return apiAddr, metricsAddr
}
