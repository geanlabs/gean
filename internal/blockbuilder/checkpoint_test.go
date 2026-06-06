package blockbuilder

import (
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func TestJustifiedMeetsRequired(t *testing.T) {
	actualRoot := [32]byte{0x11}

	tests := []struct {
		name     string
		actual   *types.Checkpoint
		required *types.Checkpoint
		history  []*types.Checkpoint
		want     bool
	}{
		{
			name:     "nil requirement",
			actual:   &types.Checkpoint{Slot: 1, Root: actualRoot},
			required: nil,
			want:     true,
		},
		{
			name:     "actual ahead",
			actual:   &types.Checkpoint{Slot: 2, Root: actualRoot},
			required: &types.Checkpoint{Slot: 1, Root: [32]byte{0x22}},
			history:  []*types.Checkpoint{{Slot: 1, Root: [32]byte{0x22}}},
			want:     true,
		},
		{
			name:     "actual ahead missing required root",
			actual:   &types.Checkpoint{Slot: 2, Root: actualRoot},
			required: &types.Checkpoint{Slot: 1, Root: [32]byte{0x22}},
			want:     false,
		},
		{
			name:     "actual ahead zero root requirement",
			actual:   &types.Checkpoint{Slot: 2, Root: actualRoot},
			required: &types.Checkpoint{Slot: 1},
			want:     true,
		},
		{
			name:     "same slot same root",
			actual:   &types.Checkpoint{Slot: 1, Root: actualRoot},
			required: &types.Checkpoint{Slot: 1, Root: actualRoot},
			want:     true,
		},
		{
			name:     "same slot zero root requirement",
			actual:   &types.Checkpoint{Slot: 1, Root: actualRoot},
			required: &types.Checkpoint{Slot: 1},
			want:     true,
		},
		{
			name:     "same slot different root",
			actual:   &types.Checkpoint{Slot: 1, Root: actualRoot},
			required: &types.Checkpoint{Slot: 1, Root: [32]byte{0x33}},
			want:     false,
		},
		{
			name:     "actual behind",
			actual:   &types.Checkpoint{Slot: 1, Root: actualRoot},
			required: &types.Checkpoint{Slot: 2, Root: actualRoot},
			want:     false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := &types.State{LatestJustified: test.actual}
			for _, checkpoint := range test.history {
				for uint64(len(state.HistoricalBlockHashes)) <= checkpoint.Slot {
					state.HistoricalBlockHashes = append(state.HistoricalBlockHashes, make([]byte, types.RootSize))
				}
				state.HistoricalBlockHashes[checkpoint.Slot] = copyRoot(checkpoint.Root)
			}
			got := justifiedMeetsRequired(state, test.required)
			if got != test.want {
				t.Fatalf("justifiedMeetsRequired()=%t, want %t", got, test.want)
			}
		})
	}
}

func TestCheckpointInHistoryRejectsMalformedRootBytes(t *testing.T) {
	root := [32]byte{0x11}
	state := &types.State{
		HistoricalBlockHashes: [][]byte{{0x11}},
	}

	if checkpointInHistory(state, &types.Checkpoint{Slot: 0, Root: root}) {
		t.Fatal("checkpointInHistory accepted short historical root bytes")
	}
}
