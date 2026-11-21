//go:build spectests

package lean

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/geanlabs/gean/internal/lean/specfixtures"
	"github.com/geanlabs/gean/internal/lean/transition"
	"github.com/geanlabs/gean/internal/lean/types"
)

func runStateTransition(t *testing.T, fixture *specfixtures.StateTransitionFixture) {
	// A fixture without a post state expects a rejection.
	expectRejection := fixture.Post == nil

	// A rejection with no blocks was raised inside the spec's own setup, for
	// example processing slots to the state's current slot. There is nothing to
	// replay.
	if len(fixture.Blocks) == 0 && expectRejection {
		return
	}

	state, err := fixture.Pre.ToState()
	if err != nil {
		t.Fatalf("pre state: %v", err)
	}
	for i, testBlock := range fixture.Blocks {
		block, err := testBlock.ToBlock()
		if err != nil {
			t.Fatalf("block %d: %v", i, err)
		}
		if err := transition.StateTransition(state, block); err != nil {
			if expectRejection {
				return
			}
			t.Fatalf("block %d (slot %d): unexpected error: %v", i, testBlock.Slot, err)
		}
	}
	if expectRejection {
		t.Fatalf("accepted every block; want rejection %s", fixture.RejectionReason)
	}

	if err := validatePostState(fixture.Post, state); err != nil {
		t.Fatalf("post state: %v", err)
	}
	if fixture.PostStateRoot != nil {
		want := mustParse(t, specfixtures.ParseHexRoot, *fixture.PostStateRoot)
		got, err := state.HashTreeRoot()
		if err != nil {
			t.Fatalf("post state root: %v", err)
		}
		if got != want {
			t.Fatalf("post state root: got 0x%x, want 0x%x", got, want)
		}
	}
}

func validatePostState(post *specfixtures.TestPostState, state *types.State) error {
	if post.Slot != nil {
		if state.Slot != *post.Slot {
			return fmt.Errorf("slot: got %d, want %d", state.Slot, *post.Slot)
		}
	}

	if post.LatestBlockHeaderSlot != nil {
		if state.LatestBlockHeader.Slot != *post.LatestBlockHeaderSlot {
			return fmt.Errorf("latestBlockHeader.slot: got %d, want %d",
				state.LatestBlockHeader.Slot, *post.LatestBlockHeaderSlot)
		}
	}

	if post.LatestBlockHeaderStateRoot != nil {
		want, err := specfixtures.ParseHexRoot(*post.LatestBlockHeaderStateRoot)
		if err != nil {
			return err
		}
		if state.LatestBlockHeader.StateRoot != want {
			return fmt.Errorf("latestBlockHeader.stateRoot: got 0x%x, want 0x%x",
				state.LatestBlockHeader.StateRoot, want)
		}
	}

	if post.LatestBlockHeaderProposerIndex != nil {
		if state.LatestBlockHeader.ProposerIndex != *post.LatestBlockHeaderProposerIndex {
			return fmt.Errorf("latestBlockHeader.proposerIndex: got %d, want %d",
				state.LatestBlockHeader.ProposerIndex, *post.LatestBlockHeaderProposerIndex)
		}
	}

	if post.LatestBlockHeaderParentRoot != nil {
		want, err := specfixtures.ParseHexRoot(*post.LatestBlockHeaderParentRoot)
		if err != nil {
			return err
		}
		if state.LatestBlockHeader.ParentRoot != want {
			return fmt.Errorf("latestBlockHeader.parentRoot: got 0x%x, want 0x%x",
				state.LatestBlockHeader.ParentRoot, want)
		}
	}

	if post.LatestBlockHeaderBodyRoot != nil {
		want, err := specfixtures.ParseHexRoot(*post.LatestBlockHeaderBodyRoot)
		if err != nil {
			return err
		}
		if state.LatestBlockHeader.BodyRoot != want {
			return fmt.Errorf("latestBlockHeader.bodyRoot: got 0x%x, want 0x%x",
				state.LatestBlockHeader.BodyRoot, want)
		}
	}

	if post.LatestJustifiedSlot != nil {
		if state.LatestJustified.Slot != *post.LatestJustifiedSlot {
			return fmt.Errorf("latestJustified.slot: got %d, want %d",
				state.LatestJustified.Slot, *post.LatestJustifiedSlot)
		}
	}

	if post.LatestJustifiedRoot != nil {
		want, err := specfixtures.ParseHexRoot(*post.LatestJustifiedRoot)
		if err != nil {
			return err
		}
		if state.LatestJustified.Root != want {
			return fmt.Errorf("latestJustified.root: got 0x%x, want 0x%x",
				state.LatestJustified.Root, want)
		}
	}

	if post.LatestFinalizedSlot != nil {
		if state.LatestFinalized.Slot != *post.LatestFinalizedSlot {
			return fmt.Errorf("latestFinalized.slot: got %d, want %d",
				state.LatestFinalized.Slot, *post.LatestFinalizedSlot)
		}
	}

	if post.LatestFinalizedRoot != nil {
		want, err := specfixtures.ParseHexRoot(*post.LatestFinalizedRoot)
		if err != nil {
			return err
		}
		if state.LatestFinalized.Root != want {
			return fmt.Errorf("latestFinalized.root: got 0x%x, want 0x%x",
				state.LatestFinalized.Root, want)
		}
	}

	if post.HistoricalBlockHashesCount != nil {
		got := uint64(len(state.HistoricalBlockHashes))
		if got != *post.HistoricalBlockHashesCount {
			return fmt.Errorf("historicalBlockHashes count: got %d, want %d",
				got, *post.HistoricalBlockHashesCount)
		}
	}

	if post.ValidatorCount != nil {
		got := state.NumValidators()
		if got != *post.ValidatorCount {
			return fmt.Errorf("validator count: got %d, want %d", got, *post.ValidatorCount)
		}
	}

	if post.ConfigGenesisTime != nil {
		if state.Config.GenesisTime != *post.ConfigGenesisTime {
			return fmt.Errorf("config.genesisTime: got %d, want %d",
				state.Config.GenesisTime, *post.ConfigGenesisTime)
		}
	}

	if post.HistoricalBlockHashes != nil {
		wantLen := len(post.HistoricalBlockHashes.Data)
		gotLen := len(state.HistoricalBlockHashes)
		if gotLen != wantLen {
			return fmt.Errorf("historicalBlockHashes length: got %d, want %d", gotLen, wantLen)
		}
		for i, raw := range post.HistoricalBlockHashes.Data {
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

	if post.JustifiedSlots != nil {
		wantBitlist, err := specfixtures.ParseBoolBitlist(post.JustifiedSlots.Data)
		if err != nil {
			return err
		}
		if !bytes.Equal(state.JustifiedSlots, wantBitlist) {
			return fmt.Errorf("justifiedSlots mismatch: got %x, want %x",
				state.JustifiedSlots, wantBitlist)
		}
	}

	if post.JustificationsRoots != nil {
		wantLen := len(post.JustificationsRoots.Data)
		gotLen := len(state.JustificationsRoots)
		if gotLen != wantLen {
			return fmt.Errorf("justificationsRoots length: got %d, want %d", gotLen, wantLen)
		}
		for i, raw := range post.JustificationsRoots.Data {
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

	if post.JustificationsRootsCount != nil {
		if got := uint64(len(state.JustificationsRoots)); got != *post.JustificationsRootsCount {
			return fmt.Errorf("justificationsRoots count: got %d, want %d", got, *post.JustificationsRootsCount)
		}
	}

	if post.JustificationsValidators != nil {
		wantBitlist, err := specfixtures.ParseBoolBitlist(post.JustificationsValidators.Data)
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
