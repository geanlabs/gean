//go:build spectests

package lean

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/geanlabs/gean/internal/lean/specfixtures"
	"github.com/geanlabs/gean/internal/lean/types"
)

// mustParse fails only the current subtest on a malformed fixture value, so one
// bad fixture does not abort the whole run.
func mustParse[T any](t *testing.T, parse func(string) (T, error), s string) T {
	t.Helper()
	v, err := parse(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

func validatePostState(tp *specfixtures.TestPostState, state *types.State) error {
	if tp.Slot != nil {
		if state.Slot != *tp.Slot {
			return fmt.Errorf("slot: got %d, want %d", state.Slot, *tp.Slot)
		}
	}

	if tp.LatestBlockHeaderSlot != nil {
		if state.LatestBlockHeader.Slot != *tp.LatestBlockHeaderSlot {
			return fmt.Errorf("latestBlockHeader.slot: got %d, want %d",
				state.LatestBlockHeader.Slot, *tp.LatestBlockHeaderSlot)
		}
	}

	if tp.LatestBlockHeaderStateRoot != nil {
		want, err := specfixtures.ParseHexRoot(*tp.LatestBlockHeaderStateRoot)
		if err != nil {
			return err
		}
		if state.LatestBlockHeader.StateRoot != want {
			return fmt.Errorf("latestBlockHeader.stateRoot: got 0x%x, want 0x%x",
				state.LatestBlockHeader.StateRoot, want)
		}
	}

	if tp.LatestBlockHeaderProposerIndex != nil {
		if state.LatestBlockHeader.ProposerIndex != *tp.LatestBlockHeaderProposerIndex {
			return fmt.Errorf("latestBlockHeader.proposerIndex: got %d, want %d",
				state.LatestBlockHeader.ProposerIndex, *tp.LatestBlockHeaderProposerIndex)
		}
	}

	if tp.LatestBlockHeaderParentRoot != nil {
		want, err := specfixtures.ParseHexRoot(*tp.LatestBlockHeaderParentRoot)
		if err != nil {
			return err
		}
		if state.LatestBlockHeader.ParentRoot != want {
			return fmt.Errorf("latestBlockHeader.parentRoot: got 0x%x, want 0x%x",
				state.LatestBlockHeader.ParentRoot, want)
		}
	}

	if tp.LatestBlockHeaderBodyRoot != nil {
		want, err := specfixtures.ParseHexRoot(*tp.LatestBlockHeaderBodyRoot)
		if err != nil {
			return err
		}
		if state.LatestBlockHeader.BodyRoot != want {
			return fmt.Errorf("latestBlockHeader.bodyRoot: got 0x%x, want 0x%x",
				state.LatestBlockHeader.BodyRoot, want)
		}
	}

	if tp.LatestJustifiedSlot != nil {
		if state.LatestJustified.Slot != *tp.LatestJustifiedSlot {
			return fmt.Errorf("latestJustified.slot: got %d, want %d",
				state.LatestJustified.Slot, *tp.LatestJustifiedSlot)
		}
	}

	if tp.LatestJustifiedRoot != nil {
		want, err := specfixtures.ParseHexRoot(*tp.LatestJustifiedRoot)
		if err != nil {
			return err
		}
		if state.LatestJustified.Root != want {
			return fmt.Errorf("latestJustified.root: got 0x%x, want 0x%x",
				state.LatestJustified.Root, want)
		}
	}

	if tp.LatestFinalizedSlot != nil {
		if state.LatestFinalized.Slot != *tp.LatestFinalizedSlot {
			return fmt.Errorf("latestFinalized.slot: got %d, want %d",
				state.LatestFinalized.Slot, *tp.LatestFinalizedSlot)
		}
	}

	if tp.LatestFinalizedRoot != nil {
		want, err := specfixtures.ParseHexRoot(*tp.LatestFinalizedRoot)
		if err != nil {
			return err
		}
		if state.LatestFinalized.Root != want {
			return fmt.Errorf("latestFinalized.root: got 0x%x, want 0x%x",
				state.LatestFinalized.Root, want)
		}
	}

	if tp.HistoricalBlockHashesCount != nil {
		got := uint64(len(state.HistoricalBlockHashes))
		if got != *tp.HistoricalBlockHashesCount {
			return fmt.Errorf("historicalBlockHashes count: got %d, want %d",
				got, *tp.HistoricalBlockHashesCount)
		}
	}

	if tp.ValidatorCount != nil {
		got := state.NumValidators()
		if got != *tp.ValidatorCount {
			return fmt.Errorf("validator count: got %d, want %d", got, *tp.ValidatorCount)
		}
	}

	if tp.ConfigGenesisTime != nil {
		if state.Config.GenesisTime != *tp.ConfigGenesisTime {
			return fmt.Errorf("config.genesisTime: got %d, want %d",
				state.Config.GenesisTime, *tp.ConfigGenesisTime)
		}
	}

	if tp.HistoricalBlockHashes != nil {
		wantLen := len(tp.HistoricalBlockHashes.Data)
		gotLen := len(state.HistoricalBlockHashes)
		if gotLen != wantLen {
			return fmt.Errorf("historicalBlockHashes length: got %d, want %d", gotLen, wantLen)
		}
		for i, raw := range tp.HistoricalBlockHashes.Data {
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				return fmt.Errorf("historicalBlockHashes[%d] unmarshal: %v", i, err)
			}
			want, err := specfixtures.ParseHexBytes(s)
			if err != nil {
				return err
			}
			got := state.HistoricalBlockHashes[i]
			if !bytes.Equal(got, want) {
				return fmt.Errorf("historicalBlockHashes[%d]: got 0x%x, want 0x%x", i, got, want)
			}
		}
	}

	if tp.JustifiedSlots != nil {
		wantBitlist, err := specfixtures.ParseBoolBitlist(tp.JustifiedSlots.Data)
		if err != nil {
			return err
		}
		if !bytes.Equal(state.JustifiedSlots, wantBitlist) {
			return fmt.Errorf("justifiedSlots mismatch: got %x, want %x",
				state.JustifiedSlots, wantBitlist)
		}
	}

	if tp.JustificationsRoots != nil {
		wantLen := len(tp.JustificationsRoots.Data)
		gotLen := len(state.JustificationsRoots)
		if gotLen != wantLen {
			return fmt.Errorf("justificationsRoots length: got %d, want %d", gotLen, wantLen)
		}
		for i, raw := range tp.JustificationsRoots.Data {
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				return fmt.Errorf("justificationsRoots[%d] unmarshal: %v", i, err)
			}
			want, err := specfixtures.ParseHexBytes(s)
			if err != nil {
				return err
			}
			got := state.JustificationsRoots[i]
			if !bytes.Equal(got, want) {
				return fmt.Errorf("justificationsRoots[%d]: got 0x%x, want 0x%x", i, got, want)
			}
		}
	}

	if tp.JustificationsValidators != nil {
		wantBitlist, err := specfixtures.ParseBoolBitlist(tp.JustificationsValidators.Data)
		if err != nil {
			return err
		}
		if !bytes.Equal(state.JustificationsValidators, wantBitlist) {
			return fmt.Errorf("justificationsValidators mismatch: got %x, want %x",
				state.JustificationsValidators, wantBitlist)
		}
	}

	return nil
}
