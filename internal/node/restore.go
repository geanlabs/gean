package node

import (
	"fmt"

	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/store"
)

// ForkChoiceFromStore anchors fork choice at the latest justified block and
// replays every imported block above it. FindHead descends from the
// justified root, so after a restart that root must be present in the array —
// anchoring at the bare DB head leaves justified as an unknown ancestor and
// pins the head there permanently. All branches are replayed, not just the
// head chain: the network may have built on a sibling the previous run never
// chose as head, and a missing fork point leaves that subtree dangling and
// weightless. Only imported blocks qualify, read from the live chain — headers
// are also persisted for pending blocks that were never verified, and an
// unverified block must not be able to win head. A stored post-state is not the
// test: states of old blocks are pruned while finalization stalls, and their
// descendants would be left dangling.
func ForkChoiceFromStore(s *store.ConsensusStore) (*forkchoice.ForkChoice, error) {
	if s == nil {
		return nil, fmt.Errorf("fork choice anchor: store is nil")
	}
	anchorRoot := s.Head()
	if justified := s.LatestJustified(); justified != nil && s.GetBlockHeader(justified.Root) != nil {
		anchorRoot = justified.Root
	}
	anchorHeader := s.GetBlockHeader(anchorRoot)
	if anchorHeader == nil {
		return nil, fmt.Errorf("fork choice anchor: missing header for root 0x%x", anchorRoot)
	}
	fc := forkchoice.New(anchorHeader.Slot, anchorRoot, anchorHeader.ParentRoot)

	imported, err := s.LiveChainRange(anchorHeader.Slot+1, 0)
	if err != nil {
		return nil, fmt.Errorf("fork choice anchor: %w", err)
	}
	for _, e := range imported {
		// A block pruned whole as a dead fork keeps its live-chain entry; its
		// missing header marks it gone.
		if s.GetBlockHeader(e.Root) == nil {
			continue
		}
		fc.OnBlock(e.Slot, e.Root, e.ParentRoot)
	}
	return fc, nil
}
