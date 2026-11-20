package main

import (
	"os"
	"syscall"
	"testing"
)

// The first signal closes stop instead of killing the process, so a signal
// during startup still goes through teardown.
func TestFirstSignalClosesStop(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	stop := shutdownOn(sigCh)
	sigCh <- syscall.SIGINT
	<-stop
}
