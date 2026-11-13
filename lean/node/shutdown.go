package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/internal/lifecycle"
	"github.com/geanlabs/gean/internal/logger"
)

// shutdownProgressInterval paces the log line for a shutdown step still
// waiting, usually on a proof that cannot be interrupted.
const shutdownProgressInterval = 5 * time.Second

// components holds what run has started. A nil field was never started, so
// teardown skips it; that makes one path serve a full shutdown and a startup
// that failed halfway.
type components struct {
	cancel       context.CancelFunc
	servers      []*httpServer
	tasks        lifecycle.Group // HTTP serving and the subscription re-announce
	closeNetwork func()
	waitSync     func()
	waitEngine   func()
	closeStore   func() error
	freeKeys     func()
	freePubKeys  func()
}

// teardown stops admission, cancels the background work, joins everything
// that uses the database, the validator keys or the decoded public keys, and
// only then closes the database and frees the keys. No step has a deadline
// that frees a resource still in use; a second signal exits without freeing
// anything.
func (c *components) teardown() {
	for _, srv := range c.servers {
		srv.shutdown()
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.tasks.Close()
	if c.closeNetwork != nil {
		waitLogged("network handlers", c.closeNetwork)
	}
	if c.waitSync != nil {
		waitLogged("sync driver", c.waitSync)
	}
	if c.waitEngine != nil {
		waitLogged("engine workers (a proof in flight cannot be interrupted)", c.waitEngine)
	}
	if c.closeStore != nil {
		if err := c.closeStore(); err != nil {
			logger.Error(logger.Node, "close database: %v", err)
		}
	}
	if c.freePubKeys != nil {
		c.freePubKeys()
	}
	c.freeKeys()
}

// waitLogged runs wait and logs every shutdownProgressInterval until it returns.
func waitLogged(what string, wait func()) {
	done := make(chan struct{})
	go func() {
		wait()
		close(done)
	}()
	ticker := time.NewTicker(shutdownProgressInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			logger.Warn(logger.Node, "shutdown: still waiting for %s", what)
		}
	}
}
