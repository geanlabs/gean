package sim

import (
	"context"
	"os"
	"testing"

	"github.com/geanlabs/gean/logger"
)

func TestMain(m *testing.M) {
	logger.SetQuiet(true)
	os.Exit(m.Run())
}

// slotView is what every node must agree on at the end of a slot.
type slotView struct {
	head      [32]byte
	headSlot  uint64
	justified uint64
	finalized uint64
}

// runScenario runs two validator nodes, one of them the aggregator, and an
// observer node holding no keys, and returns the observer's view of each slot
// after checking that every node agrees with it.
func runScenario(t *testing.T, slots int) []slotView {
	t.Helper()
	ctx := context.Background()
	c, err := New(ctx, Config{
		GenesisTime: 1_000_000,
		Validators:  4,
		Nodes:       [][]uint64{{0, 1}, {2, 3}, {}},
		Aggregators: []int{0},
	})
	if err != nil {
		t.Fatalf("new cluster: %v", err)
	}
	defer c.Close()

	var trace []slotView
	for range slots {
		c.AdvanceSlots(ctx, 1)
		var want slotView
		for i, n := range c.Nodes() {
			got := slotView{
				head:      n.Store.Head(),
				headSlot:  n.Store.HeadSlot(),
				justified: n.Store.LatestJustified().Slot,
				finalized: n.Store.LatestFinalized().Slot,
			}
			if i == 0 {
				want = got
			} else if got != want {
				t.Fatalf("slot %d: node %d view %+v differs from node 0 %+v", c.Slot(), i, got, want)
			}
		}
		if want.headSlot != c.Slot() {
			t.Fatalf("slot %d: head slot %d, want a block every slot", c.Slot(), want.headSlot)
		}
		trace = append(trace, want)
	}
	return trace
}

func TestClusterAgreesAndFinalizesDeterministically(t *testing.T) {
	const slots = 8
	first := runScenario(t, slots)
	if last := first[len(first)-1]; last.finalized == 0 || last.justified <= last.finalized {
		t.Fatalf("after %d slots justified=%d finalized=%d, want finality to advance", slots, last.justified, last.finalized)
	}

	second := runScenario(t, slots)
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("slot %d: second run %+v differs from first %+v", i+1, second[i], first[i])
		}
	}
}

// Aggregation can be switched on at runtime through the admin API, so a node
// built without the role must still be able to take it on. Without an
// aggregator no votes are aggregated and justification stalls; enabling the
// role on a running node must restart justification at once and finality once
// a justifiable slot distance allows it.
func TestRuntimeAggregatorRestartsFinality(t *testing.T) {
	ctx := context.Background()
	c, err := New(ctx, Config{
		GenesisTime: 1_000_000,
		Validators:  4,
		Nodes:       [][]uint64{{0, 1}, {2, 3}},
	})
	if err != nil {
		t.Fatalf("new cluster: %v", err)
	}
	defer c.Close()

	c.AdvanceSlots(ctx, 6)
	if got := c.Nodes()[0].Store.LatestJustified().Slot; got != 0 {
		t.Fatalf("justified slot %d without an aggregator, want 0", got)
	}

	c.Nodes()[1].Engine.AggCtl.Set(true)
	c.AdvanceSlots(ctx, 1)
	if got := c.Nodes()[0].Store.LatestJustified().Slot; got == 0 {
		t.Fatal("justification did not resume in the slot after enabling an aggregator")
	}
	c.AdvanceSlots(ctx, 7)
	for i, n := range c.Nodes() {
		if got := n.Store.LatestFinalized().Slot; got == 0 {
			t.Fatalf("node %d: finality did not resume within 8 slots of enabling an aggregator", i)
		}
	}
}
