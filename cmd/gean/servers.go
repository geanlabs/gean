package main

import (
	"context"

	"github.com/geanlabs/gean/internal/api"
	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/tasks"
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
