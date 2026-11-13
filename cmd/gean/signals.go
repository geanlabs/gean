package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/geanlabs/gean/internal/logger"
)

// notifyShutdown returns a channel closed on the first SIGINT or SIGTERM. It is
// installed before startup, so a signal during a long startup step is honoured
// once the step returns. A second signal exits at once without freeing
// anything, which is safe because the process is ending.
func notifyShutdown() <-chan struct{} {
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	return shutdownOn(sigCh)
}

// shutdownOn closes the returned channel on the first signal from sigCh and
// exits on the second.
func shutdownOn(sigCh <-chan os.Signal) <-chan struct{} {
	stop := make(chan struct{})
	go func() {
		sig := <-sigCh
		logger.Info(logger.Node, "shutting down on %s; signal again to exit without waiting", sig)
		close(stop)
		<-sigCh
		logger.Warn(logger.Node, "second signal: exiting without waiting for shutdown")
		os.Exit(1)
	}()
	return stop
}
