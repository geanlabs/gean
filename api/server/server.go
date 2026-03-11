package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"syscall"
	"time"

	"github.com/geanlabs/gean/api/httprest"
	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/observability/logging"
)

// StoreGetter returns the current forkchoice store.
type StoreGetter func() *forkchoice.Store

// Server is a lightweight HTTP API server.
type Server struct {
	cfg         Config
	storeGetter StoreGetter
	httpServer  *http.Server
	log         *slog.Logger
}

// New constructs a new API server.
func New(cfg Config, storeGetter StoreGetter) *Server {
	return &Server{
		cfg:         cfg,
		storeGetter: storeGetter,
		log:         logging.NewComponentLogger(logging.CompAPI),
	}
}

// Start launches the HTTP server in the background.
func (s *Server) Start() error {
	if !s.cfg.Enabled {
		return nil
	}
	if s.httpServer != nil {
		return nil
	}

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	mux := httprest.NewMux(s.storeGetter)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			s.log.Warn("api server already running; skipping", "addr", addr)
			return nil
		}
		return err
	}

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("api server error", "err", err)
		}
	}()

	s.log.Info("api server started", "addr", addr)
	return nil
}

// Stop gracefully shuts down the HTTP server.
func (s *Server) Stop() {
	if s.httpServer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.httpServer.Shutdown(ctx)
	s.httpServer = nil
	s.log.Info("api server stopped")
}
