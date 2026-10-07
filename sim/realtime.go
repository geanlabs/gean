package sim

import (
	"context"

	"github.com/geanlabs/gean/net/syncer"
	"github.com/geanlabs/gean/node"
	"github.com/geanlabs/gean/tasks"
)

// RealTime is a cluster whose engines run with Run on the system clock, as
// the binary does: each node's workers run on their own goroutines, their
// results arrive whenever they finish, the prover gate is contended for real,
// and each node runs the sync driver over the simulated network. It is not deterministic; use it for properties that must hold under
// real concurrency, and New for exact scenarios.
type RealTime struct {
	cluster *Cluster
	cancel  context.CancelFunc
	engines tasks.Group
}

// StartRealTime builds the cluster on the system clock and starts every
// engine. Set cfg.GenesisTime near the current time so slots start at once.
// It takes ownership of cfg.Crypto as New does.
func StartRealTime(ctx context.Context, cfg Config) (*RealTime, error) {
	c, err := build(cfg, node.SystemClock{})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	r := &RealTime{cluster: c, cancel: cancel}
	for _, n := range c.nodes {
		r.engines.Go(func() { n.Engine.Run(ctx) })
		syncDriver := syncer.NewSyncDriver(ctx, n.Engine, n.Store, n.network)
		r.engines.Go(syncDriver.Run)
	}
	return r, nil
}

// Nodes returns the nodes in index order.
func (r *RealTime) Nodes() []*Node { return r.cluster.nodes }

// Stop cancels every engine, waits until all of their work has finished, then
// releases the cluster's crypto.
func (r *RealTime) Stop() {
	r.cancel()
	r.engines.Wait()
	r.cluster.Close()
}
