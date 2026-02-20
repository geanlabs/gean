//go:build skip_sig_verify

package spectests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/types"
)

const stfFixtureDir = "../../leanSpec/fixtures/consensus/state_transition"

func TestStateTransition(t *testing.T) {
	files := findJSONFiles(t, stfFixtureDir)

	for _, file := range files {
		file := file
		relPath, _ := filepath.Rel(stfFixtureDir, file)
		t.Run(relPath, func(t *testing.T) {
			runStateTransitionFixture(t, file)
		})
	}
}

func findJSONFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".json" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk fixture directory %s: %v", root, err)
	}
	if len(files) == 0 {
		t.Fatalf("no fixture files found in %s — run 'make leanSpec/fixtures' first", root)
	}
	return files
}

func runStateTransitionFixture(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	var fixture StateTransitionFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("failed to unmarshal fixture: %v", err)
	}

	for testName, tc := range fixture {
		tc := tc
		t.Run(testName, func(t *testing.T) {
			if tc.Info.FixtureFormat != "state_transition_test" {
				t.Skipf("unsupported fixture format: %s", tc.Info.FixtureFormat)
			}

			state := convertState(tc.Pre)
			expectFailure := tc.ExpectException != nil || tc.Post == nil

			var transitionErr error
			for _, fb := range tc.Blocks {
				block := convertBlock(fb)
				state, transitionErr = statetransition.StateTransition(state, block)
				if transitionErr != nil {
					break
				}
			}

			if expectFailure {
				if transitionErr == nil && len(tc.Blocks) > 0 {
					t.Fatalf("[%s] expected failure but state transition succeeded", testName)
				}
				return
			}

			if transitionErr != nil {
				t.Fatalf("[%s] unexpected state transition error: %v", testName, transitionErr)
			}

			validatePostState(t, testName, state, tc.Post)
		})
	}
}

func validatePostState(t *testing.T, testName string, state *types.State, post *PostState) {
	t.Helper()
	if post == nil {
		return
	}

	check := func(field string, got, want interface{}) {
		if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
			t.Errorf("[%s] %s mismatch: got %v, want %v", testName, field, got, want)
		}
	}

	if post.Slot != nil {
		check("slot", state.Slot, *post.Slot)
	}
	if post.LatestJustifiedSlot != nil {
		check("latestJustified.slot", state.LatestJustified.Slot, *post.LatestJustifiedSlot)
	}
	if post.LatestJustifiedRoot != nil {
		check("latestJustified.root", state.LatestJustified.Root, [32]byte(*post.LatestJustifiedRoot))
	}
	if post.LatestFinalizedSlot != nil {
		check("latestFinalized.slot", state.LatestFinalized.Slot, *post.LatestFinalizedSlot)
	}
	if post.LatestFinalizedRoot != nil {
		check("latestFinalized.root", state.LatestFinalized.Root, [32]byte(*post.LatestFinalizedRoot))
	}
	if post.ValidatorCount != nil {
		check("validatorCount", uint64(len(state.Validators)), *post.ValidatorCount)
	}
	if post.ConfigGenesisTime != nil {
		check("config.genesisTime", state.Config.GenesisTime, *post.ConfigGenesisTime)
	}
	if post.LatestBlockHeaderSlot != nil {
		check("latestBlockHeader.slot", state.LatestBlockHeader.Slot, *post.LatestBlockHeaderSlot)
	}
	if post.LatestBlockHeaderProposerIndex != nil {
		check("latestBlockHeader.proposerIndex", state.LatestBlockHeader.ProposerIndex, *post.LatestBlockHeaderProposerIndex)
	}
	if post.LatestBlockHeaderParentRoot != nil {
		check("latestBlockHeader.parentRoot", state.LatestBlockHeader.ParentRoot, [32]byte(*post.LatestBlockHeaderParentRoot))
	}
	if post.LatestBlockHeaderStateRoot != nil {
		check("latestBlockHeader.stateRoot", state.LatestBlockHeader.StateRoot, [32]byte(*post.LatestBlockHeaderStateRoot))
	}
	if post.LatestBlockHeaderBodyRoot != nil {
		check("latestBlockHeader.bodyRoot", state.LatestBlockHeader.BodyRoot, [32]byte(*post.LatestBlockHeaderBodyRoot))
	}
	if post.HistoricalBlockHashesCount != nil {
		check("historicalBlockHashes.count", uint64(len(state.HistoricalBlockHashes)), *post.HistoricalBlockHashesCount)
	}
	if post.HistoricalBlockHashes != nil {
		expected := make([][32]byte, len(post.HistoricalBlockHashes.Data))
		for i, h := range post.HistoricalBlockHashes.Data {
			expected[i] = [32]byte(h)
		}
		if len(state.HistoricalBlockHashes) != len(expected) {
			t.Errorf("[%s] historicalBlockHashes length mismatch: got %d, want %d",
				testName, len(state.HistoricalBlockHashes), len(expected))
		} else {
			for i := range expected {
				if state.HistoricalBlockHashes[i] != expected[i] {
					t.Errorf("[%s] historicalBlockHashes[%d] mismatch: got %x, want %x",
						testName, i, state.HistoricalBlockHashes[i], expected[i])
				}
			}
		}
	}
	if post.JustifiedSlots != nil {
		expectedBitlist := buildBitlist(post.JustifiedSlots.Data)
		actualLen := statetransition.BitlistLen(state.JustifiedSlots)
		expectedLen := statetransition.BitlistLen(expectedBitlist)
		if actualLen != expectedLen {
			t.Errorf("[%s] justifiedSlots length mismatch: got %d bits, want %d bits",
				testName, actualLen, expectedLen)
		} else {
			for i := 0; i < actualLen; i++ {
				a := statetransition.GetBit(state.JustifiedSlots, uint64(i))
				e := statetransition.GetBit(expectedBitlist, uint64(i))
				if a != e {
					t.Errorf("[%s] justifiedSlots[%d] mismatch: got %v, want %v",
						testName, i, a, e)
				}
			}
		}
	}
	if post.JustificationsRoots != nil {
		expected := make([][32]byte, len(post.JustificationsRoots.Data))
		for i, r := range post.JustificationsRoots.Data {
			expected[i] = [32]byte(r)
		}
		if len(state.JustificationsRoots) != len(expected) {
			t.Errorf("[%s] justificationsRoots length mismatch: got %d, want %d",
				testName, len(state.JustificationsRoots), len(expected))
		} else {
			for i := range expected {
				if state.JustificationsRoots[i] != expected[i] {
					t.Errorf("[%s] justificationsRoots[%d] mismatch: got %x, want %x",
						testName, i, state.JustificationsRoots[i], expected[i])
				}
			}
		}
	}
	if post.JustificationsValidators != nil {
		expectedBitlist := buildBoolBitlist(post.JustificationsValidators.Data)
		actualLen := statetransition.BitlistLen(state.JustificationsValidators)
		expectedLen := statetransition.BitlistLen(expectedBitlist)
		if actualLen != expectedLen {
			t.Errorf("[%s] justificationsValidators length mismatch: got %d bits, want %d bits",
				testName, actualLen, expectedLen)
		} else {
			for i := 0; i < actualLen; i++ {
				a := statetransition.GetBit(state.JustificationsValidators, uint64(i))
				e := statetransition.GetBit(expectedBitlist, uint64(i))
				if a != e {
					t.Errorf("[%s] justificationsValidators[%d] mismatch: got %v, want %v",
						testName, i, a, e)
				}
			}
		}
	}
}
