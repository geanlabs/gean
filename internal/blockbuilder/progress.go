package blockbuilder

import (
	"bytes"

	"github.com/geanlabs/gean/internal/types"
)

type progressSnapshot struct {
	justified      checkpointSnapshot
	finalized      checkpointSnapshot
	justifiedSlots []byte
}

type checkpointSnapshot struct {
	root [32]byte
	slot uint64
	ok   bool
}

func captureProgress(state *types.State) progressSnapshot {
	if state == nil {
		return progressSnapshot{}
	}
	return progressSnapshot{
		justified:      captureCheckpoint(state.LatestJustified),
		finalized:      captureCheckpoint(state.LatestFinalized),
		justifiedSlots: append([]byte(nil), state.JustifiedSlots...),
	}
}

func captureCheckpoint(checkpoint *types.Checkpoint) checkpointSnapshot {
	if checkpoint == nil {
		return checkpointSnapshot{}
	}
	return checkpointSnapshot{
		root: checkpoint.Root,
		slot: checkpoint.Slot,
		ok:   true,
	}
}

func progressChanged(a, b progressSnapshot) bool {
	return a.justified != b.justified ||
		a.finalized != b.finalized ||
		!bytes.Equal(a.justifiedSlots, b.justifiedSlots)
}
