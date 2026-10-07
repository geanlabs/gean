package node

import "testing"

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
