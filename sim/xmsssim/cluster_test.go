package xmsssim

import (
	"context"
	"os"
	"testing"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/sim"
)

func TestMain(m *testing.M) {
	logger.SetQuiet(true)
	os.Exit(m.Run())
}

// The same scenario the insecure scheme runs, on real XMSS keys and proofs:
// every node agrees on each head and the chain finalizes.
func TestClusterFinalizesWithXMSS(t *testing.T) {
	ctx := context.Background()
	keys, err := New(4)
	if err != nil {
		t.Fatalf("generate keys: %v", err)
	}
	c, err := sim.New(ctx, sim.Config{
		GenesisTime: 1_000_000,
		Validators:  4,
		Nodes:       [][]uint64{{0, 1}, {2, 3}, {}},
		Aggregators: []int{0},
		Crypto:      keys,
	})
	if err != nil {
		t.Fatalf("new cluster: %v", err)
	}
	defer c.Close()

	for range 8 {
		c.AdvanceSlots(ctx, 1)
		head := c.Nodes()[0].Store.Head()
		for i, n := range c.Nodes() {
			if got := n.Store.Head(); got != head {
				t.Fatalf("slot %d: node %d head %x differs from node 0 %x", c.Slot(), i, got, head)
			}
		}
		if got := c.Nodes()[0].Store.HeadSlot(); got != c.Slot() {
			t.Fatalf("slot %d: head slot %d, want a block every slot", c.Slot(), got)
		}
	}
	if got := c.Nodes()[0].Store.LatestFinalized().Slot; got == 0 {
		t.Fatal("chain did not finalize with XMSS")
	}
}
