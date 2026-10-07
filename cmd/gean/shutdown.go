package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/geanlabs/gean/internal/logger"
)

func waitForShutdown(cancel context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	signal.Stop(sigCh)

	logger.Info(logger.Node, "shutting down...")
	cancel()
}
