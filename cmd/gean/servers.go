package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/geanlabs/gean/internal/api"
	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/lifecycle"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/store"
)

// metricsDrainTimeout bounds the metrics server's drain. Its handlers read
// neither the database nor the keys, so cutting one off frees nothing in use.
const metricsDrainTimeout = time.Second

// httpServer is a server bound to its address.
type httpServer struct {
	name     string
	server   *http.Server
	listener net.Listener
	// drainTimeout bounds shutdown; zero waits for every active request, for a
	// server whose handlers use the database.
	drainTimeout time.Duration
}

// listenHTTP binds the API and metrics addresses, so a taken port fails
// startup instead of being logged after "gean started".
func listenHTTP(cfg config, s *store.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *role.Controller) ([]*httpServer, error) {
	servers := []*httpServer{
		{name: "api", server: newAPIServer(s, fc, aggCtl)},
		{name: "metrics", server: api.NewMetricsServer(), drainTimeout: metricsDrainTimeout},
	}
	addrs := []string{cfg.apiAddress(), cfg.metricsAddress()}
	for i, srv := range servers {
		listener, err := net.Listen("tcp", addrs[i])
		if err != nil {
			for _, bound := range servers[:i] {
				bound.listener.Close()
			}
			return nil, fmt.Errorf("%s server listen %s: %w", srv.name, addrs[i], err)
		}
		srv.listener = listener
		logger.Info(logger.Network, "%s server listening on %s", srv.name, addrs[i])
	}
	return servers, nil
}

// serveHTTP serves each server on tasks. A server that stops for any reason
// other than shutdown sends its error on the returned channel.
func serveHTTP(servers []*httpServer, tasks *lifecycle.Group) <-chan error {
	failed := make(chan error, len(servers))
	for _, srv := range servers {
		tasks.Go(func() {
			if err := srv.server.Serve(srv.listener); !errors.Is(err, http.ErrServerClosed) {
				failed <- fmt.Errorf("%s server: %w", srv.name, err)
			}
		})
	}
	return failed
}

// shutdown stops accepting connections and drains active requests.
func (srv *httpServer) shutdown() {
	ctx := context.Background()
	if srv.drainTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, srv.drainTimeout)
		defer cancel()
	}
	waitLogged(srv.name+" requests", func() {
		if err := srv.server.Shutdown(ctx); err != nil {
			srv.server.Close()
		}
	})
}
