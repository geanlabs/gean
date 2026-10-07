package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/geanlabs/gean/logger"
)

// Serve serves handler on address until ctx is cancelled, then shuts the server
// down gracefully. It returns only after in-flight requests have finished, so
// the caller can then release what the handlers read.
func Serve(ctx context.Context, name, address string, handler http.Handler) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("%s listen: %w", name, err)
	}
	server := &http.Server{Handler: handler}
	logger.Info(logger.Network, "%s server listening on %s", name, address)

	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	if err := server.Shutdown(context.Background()); err != nil {
		return fmt.Errorf("%s shutdown: %w", name, err)
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
