//go:build sim

package sim

import (
	"testing"
	"time"
)

// Two lean nodes on the in-memory network connect and exchange blocks. Each
// runs one of the two validators, so a node holding blocks from both
// proposers imported the other node's blocks over the network.
func TestTwoNodesExchangeBlocks(t *testing.T) {
	s := Start(t, Config{Nodes: 2, Validators: 2, GenesisDelay: 5 * time.Second})

	Eventually(t, 90*time.Second, "both nodes to hold blocks from both proposers", func() bool {
		for _, n := range s.Nodes {
			fc, err := n.ForkChoice()
			if err != nil {
				return false
			}
			proposers := make(map[uint64]bool)
			for _, b := range fc.Nodes {
				if b.Slot > 0 {
					proposers[b.ProposerIndex] = true
				}
			}
			if len(proposers) < 2 {
				return false
			}
		}
		return true
	})
}
