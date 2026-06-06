package api

import (
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func StartMetricsServer(address string) error {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("metrics listen: %w", err)
	}

	logger.Info(logger.Network, "metrics server listening on %s", address)
	return http.Serve(listener, mux)
}
