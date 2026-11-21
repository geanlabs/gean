package lean

import (
	"time"

	"github.com/geanlabs/gean/internal/core/httpapi"
	"github.com/geanlabs/gean/internal/lean/api"
	"github.com/geanlabs/gean/internal/lean/forkchoice"
	"github.com/geanlabs/gean/internal/lean/role"
	"github.com/geanlabs/gean/internal/lean/store"
)

// metricsDrainTimeout bounds the metrics server's drain. Its handlers read
// neither the database nor the keys, so cutting one off frees nothing in use.
const metricsDrainTimeout = time.Second

// listenHTTP binds the API and metrics addresses, so a taken port fails
// startup instead of being logged after "gean started".
func listenHTTP(cfg Config, s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) ([]*httpapi.Server, error) {
	servers := []*httpapi.Server{
		httpapi.New("api", cfg.apiAddress(), api.Handler(s, fc, aggCtl), 0),
		httpapi.New("metrics", cfg.metricsAddress(), httpapi.MetricsHandler(), metricsDrainTimeout),
	}
	if err := httpapi.Listen(servers); err != nil {
		return nil, err
	}
	return servers, nil
}
