package node

import (
	"testing"

	"github.com/geanlabs/gean/types"
)

// The API reads fork choice from HTTP goroutines while the dispatch loop
// mutates it. Reading the live fork choice there was a data race; readers must
// only see published views, which nothing mutates. Run with -race.
func TestForkChoiceViewIsSafeToReadDuringUpdates(t *testing.T) {
	e := makeTestEngine()
	genesis := e.store.Head()

	done := make(chan struct{})
	go func() {
		defer close(done)
		parent := genesis
		for i := range 100 {
			root := [32]byte{0x02, byte(i)}
			e.forkChoice.OnBlock(uint64(i+1), root, parent)
			e.updateHead()
			parent = root
		}
	}()

	for {
		select {
		case <-done:
			if got := len(e.ForkChoiceView().Nodes); got != 101 {
				t.Fatalf("view has %d nodes after 100 blocks, want 101", got)
			}
			return
		default:
			for _, n := range e.ForkChoiceView().Nodes {
				_ = n.Weight
			}
		}
	}
}

// The API reports from the view alone, because pruning may delete the stored
// header and state it would otherwise read. The view must carry what it needs.
func TestForkChoiceViewCarriesProposerAndValidatorCount(t *testing.T) {
	e := makeTestEngine()
	genesis := e.store.Head()
	root := [32]byte{0x03}
	e.store.InsertBlockHeader(root, &types.BlockHeader{Slot: 1, ProposerIndex: 3, ParentRoot: genesis})
	headState := e.store.GetState(genesis)
	headState.Validators = []*types.Validator{{Index: 0}, {Index: 1}}
	e.store.InsertState(root, headState)
	e.forkChoice.OnBlock(1, root, genesis)
	e.updateHead()

	view := e.ForkChoiceView()
	var proposer uint64
	for _, n := range view.Nodes {
		if n.Root == root {
			proposer = n.ProposerIndex
		}
	}
	if proposer != 3 {
		t.Fatalf("view proposer=%d, want 3", proposer)
	}
	if view.Head != root || view.ValidatorCount != 2 {
		t.Fatalf("view head=%x validator count=%d, want head %x and 2 validators", view.Head, view.ValidatorCount, root)
	}
}

// Subscribers see every view as it is published, and one that stops reading
// never holds up the engine: its views are dropped instead.
func TestSubscribeViewsDeliversWithoutBlocking(t *testing.T) {
	e := makeTestEngine()
	views, unsubscribe := e.SubscribeViews(1)
	defer unsubscribe()
	stalled, unsubscribeStalled := e.SubscribeViews(0)
	defer unsubscribeStalled()

	genesis := e.store.Head()
	root := [32]byte{0x04}
	e.store.InsertBlockHeader(root, &types.BlockHeader{Slot: 1, ParentRoot: genesis})
	e.forkChoice.OnBlock(1, root, genesis)
	e.updateHead() // must return although stalled is never read

	select {
	case view := <-views:
		if view.Head != root {
			t.Fatalf("view head=%x, want %x", view.Head, root)
		}
	default:
		t.Fatal("subscriber received no view")
	}
	select {
	case <-stalled:
		t.Fatal("an unbuffered subscriber that was not reading received a view")
	default:
	}
}
