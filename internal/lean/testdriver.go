package lean

import (
	"context"

	"github.com/geanlabs/gean/internal/core/httpapi"
	"github.com/geanlabs/gean/internal/core/lifecycle"
	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/lean/api"
)

// runTestDriver serves only the hive test-driver routes and the health check
// on the API address until ctx ends. The hive lean simulator replays leanSpec
// fixtures through them, so no store, network or keys start, and nothing a
// running node does can disturb a replay.
func runTestDriver(ctx context.Context, cfg Config) error {
	servers := []*httpapi.Server{httpapi.New("api", cfg.apiAddress(), api.TestDriverHandler(), 0)}
	if err := httpapi.Listen(servers); err != nil {
		return err
	}
	var tasks lifecycle.Group
	failed := httpapi.Serve(servers, &tasks)
	logger.Info(logger.Node, "serving only the hive test driver")

	var err error
	select {
	case <-ctx.Done():
	case err = <-failed:
	}
	servers[0].Shutdown()
	tasks.Close()
	return err
}
