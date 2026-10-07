package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/geanlabs/gean/launch"
)

// stopOnSignal stops the node on the first SIGINT or SIGTERM. A second signal
// gets the default behaviour and kills the process.
func stopOnSignal(n *launch.Node) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	signal.Stop(sigCh)
	n.Stop()
}
