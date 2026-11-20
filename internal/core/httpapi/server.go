// Package httpapi serves a node's HTTP endpoints: it binds every server before
// the node reports it has started, serves them on tracked goroutines, drains
// them on shutdown, and writes JSON and SSZ responses. The routes are the
// chain's.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/geanlabs/gean/internal/core/lifecycle"
	"github.com/geanlabs/gean/internal/core/logger"
)

// Server is an HTTP server for one address.
type Server struct {
	Name     string
	addr     string
	server   *http.Server
	listener net.Listener
	// drainTimeout bounds shutdown; zero waits for every active request, for a
	// server whose handlers use the database.
	drainTimeout time.Duration
}

// New returns a server named name that will serve handler on addr. A non-zero
// drainTimeout bounds how long shutdown waits for active requests; zero waits
// for all of them, for a server whose handlers use the database.
func New(name, addr string, handler http.Handler, drainTimeout time.Duration) *Server {
	return &Server{
		Name:         name,
		addr:         addr,
		server:       &http.Server{Handler: handler},
		drainTimeout: drainTimeout,
	}
}

// Listen binds every server's address, so a taken port fails startup instead
// of being logged after it. On failure the addresses already bound are
// released.
func Listen(servers []*Server) error {
	for i, srv := range servers {
		listener, err := net.Listen("tcp", srv.addr)
		if err != nil {
			for _, bound := range servers[:i] {
				bound.listener.Close()
			}
			return fmt.Errorf("%s server listen %s: %w", srv.Name, srv.addr, err)
		}
		srv.listener = listener
		logger.Info(logger.Network, "%s server listening on %s", srv.Name, srv.addr)
	}
	return nil
}

// Serve serves each bound server on tasks. A server that stops for any reason
// other than shutdown sends its error on the returned channel.
func Serve(servers []*Server, tasks *lifecycle.Group) <-chan error {
	failed := make(chan error, len(servers))
	for _, srv := range servers {
		tasks.Go(func() {
			if err := srv.server.Serve(srv.listener); !errors.Is(err, http.ErrServerClosed) {
				failed <- fmt.Errorf("%s server: %w", srv.Name, err)
			}
		})
	}
	return failed
}

// Shutdown stops accepting connections and drains active requests.
func (srv *Server) Shutdown() {
	ctx := context.Background()
	if srv.drainTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, srv.drainTimeout)
		defer cancel()
	}
	if err := srv.server.Shutdown(ctx); err != nil {
		srv.server.Close()
	}
}
