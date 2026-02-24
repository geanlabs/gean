//go:build skip_sig_verify

package spectests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/storage/memory"
	"github.com/geanlabs/gean/types"
)

const fcFixtureDir = "../leanSpec/fixtures/consensus/fork_choice"

func TestForkChoice(t *testing.T) {
	files := findJSONFiles(t, fcFixtureDir)

	for _, file := range files {
		file := file
		relPath, _ := filepath.Rel(fcFixtureDir, file)
		t.Run(relPath, func(t *testing.T) {
			runForkChoiceFixture(t, file)
		})
	}
}

// findJSONFiles is defined in stf_spectests_test.go

func runForkChoiceFixture(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	var fixture ForkChoiceFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("failed to unmarshal fixture: %v", err)
	}

	for testName, tc := range fixture {
		tc := tc
		t.Run(testName, func(t *testing.T) {
			if tc.Info.FixtureFormat != "fork_choice_test" {
				t.Skipf("unsupported fixture format: %s", tc.Info.FixtureFormat)
			}

			anchorState := convertState(tc.AnchorState)
			anchorBlock := convertBlock(tc.AnchorBlock)

			store := forkchoice.NewStore(anchorState, anchorBlock, memory.New())
			genesisTime := anchorState.Config.GenesisTime

			// Block registry for label→root resolution.
			blockRegistry := make(map[string][32]byte)

			for stepIdx, step := range tc.Steps {
				var currentBlockRoot *[32]byte
				switch step.StepType {
				case "block":
					if step.Block == nil {
						t.Fatalf("[%s] step %d: block step missing block data", testName, stepIdx)
					}
					blockRoot := processBlockStep(t, testName, stepIdx, store, step, blockRegistry, genesisTime)
					currentBlockRoot = &blockRoot

				case "tick":
					if step.Time == nil {
						t.Fatalf("[%s] step %d: tick step missing time", testName, stepIdx)
					}
					store.AdvanceTime(*step.Time, false)

				case "attestation":
					if step.Attestation == nil {
						t.Fatalf("[%s] step %d: attestation step missing attestation data", testName, stepIdx)
					}
					sa := convertSignedAttestation(*step.Attestation)
					store.ProcessAttestation(sa)

				default:
					t.Fatalf("[%s] step %d: unsupported step type %q", testName, stepIdx, step.StepType)
				}

				// Validate post-step checks.
				if step.Checks != nil {
					validateStoreChecks(t, testName, stepIdx, store, step.Checks, blockRegistry, currentBlockRoot)
				}
			}
		})
	}
}

func processBlockStep(t *testing.T, testName string, stepIdx int, store *forkchoice.Store, step ForkChoiceStep, blockRegistry map[string][32]byte, genesisTime uint64) [32]byte {
	t.Helper()

	block := convertBlock(step.Block.Block)
	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		t.Fatalf("[%s] step %d: failed to compute block root: %v", testName, stepIdx, err)
	}

	// Advance time to the block's slot before processing.
	blockTime := block.Slot*types.SecondsPerSlot + genesisTime
	store.AdvanceTime(blockTime, true)

	// Build the signed block envelope.
	var proposerAtt *types.Attestation
	sigCount := len(block.Body.Attestations)
	if step.Block.ProposerAttestation != nil {
		proposerAtt = convertAttestation(*step.Block.ProposerAttestation)
		sigCount++
	}

	envelope := &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{
			Block:               block,
			ProposerAttestation: proposerAtt,
		},
		Signature: makeZeroSignatures(sigCount),
	}

	err = store.ProcessBlock(envelope)

	if step.Valid {
		if err != nil {
			t.Fatalf("[%s] step %d: expected valid block but got error: %v", testName, stepIdx, err)
		}
	} else {
		if err == nil {
			t.Fatalf("[%s] step %d: expected invalid block but processing succeeded", testName, stepIdx)
		}
	}

	return blockRoot
}

func validateStoreChecks(t *testing.T, testName string, stepIdx int, store *forkchoice.Store, checks *StoreChecks, blockRegistry map[string][32]byte, currentBlockRoot *[32]byte) {
	t.Helper()

	status := store.GetStatus()
	justifiedRoot := status.JustifiedRoot

	if checks.HeadSlot != nil {
		if status.HeadSlot != *checks.HeadSlot {
			t.Errorf("[%s] step %d: headSlot mismatch: got %d, want %d",
				testName, stepIdx, status.HeadSlot, *checks.HeadSlot)
		}
	}

	if checks.HeadRoot != nil {
		expected := [32]byte(*checks.HeadRoot)
		if status.Head != expected {
			t.Errorf("[%s] step %d: headRoot mismatch: got %x, want %x",
				testName, stepIdx, status.Head, expected)
		}
	}

	if checks.HeadRootLabel != nil {
		label := *checks.HeadRootLabel
		labelRoot := status.Head
		if checks.HeadRoot != nil {
			labelRoot = [32]byte(*checks.HeadRoot)
		}
		existingRoot, exists := blockRegistry[label]
		if !exists {
			blockRegistry[label] = labelRoot
		} else if existingRoot != labelRoot {
			t.Errorf("[%s] step %d: headRootLabel %q remapped: got %x, want %x",
				testName, stepIdx, label, labelRoot, existingRoot)
		}
		if status.Head != blockRegistry[label] {
			t.Errorf("[%s] step %d: headRootLabel %q mismatch: got %x, want %x",
				testName, stepIdx, label, status.Head, blockRegistry[label])
		}
	}

	if checks.LatestJustifiedSlot != nil {
		if status.JustifiedSlot != *checks.LatestJustifiedSlot {
			t.Errorf("[%s] step %d: latestJustified.slot mismatch: got %d, want %d",
				testName, stepIdx, status.JustifiedSlot, *checks.LatestJustifiedSlot)
		}
	}

	if checks.LatestJustifiedRoot != nil {
		expected := [32]byte(*checks.LatestJustifiedRoot)
		if justifiedRoot != expected {
			t.Errorf("[%s] step %d: latestJustified.root mismatch: got %x, want %x",
				testName, stepIdx, justifiedRoot, expected)
		}
	}
	if checks.LatestJustifiedRootLabel != nil {
		label := *checks.LatestJustifiedRootLabel
		labelRoot := justifiedRoot
		if checks.LatestJustifiedRoot != nil {
			labelRoot = [32]byte(*checks.LatestJustifiedRoot)
		}
		existingRoot, exists := blockRegistry[label]
		if !exists {
			blockRegistry[label] = labelRoot
		} else if existingRoot != labelRoot {
			t.Errorf("[%s] step %d: latestJustifiedRootLabel %q remapped: got %x, want %x",
				testName, stepIdx, label, labelRoot, existingRoot)
		}
		if justifiedRoot != blockRegistry[label] {
			t.Errorf("[%s] step %d: latestJustifiedRootLabel %q mismatch: got %x, want %x",
				testName, stepIdx, label, justifiedRoot, blockRegistry[label])
		}
	}

	if checks.LatestFinalizedSlot != nil {
		if status.FinalizedSlot != *checks.LatestFinalizedSlot {
			t.Errorf("[%s] step %d: latestFinalized.slot mismatch: got %d, want %d",
				testName, stepIdx, status.FinalizedSlot, *checks.LatestFinalizedSlot)
		}
	}

	if checks.LatestFinalizedRoot != nil {
		expected := [32]byte(*checks.LatestFinalizedRoot)
		if status.FinalizedRoot != expected {
			t.Errorf("[%s] step %d: latestFinalized.root mismatch: got %x, want %x",
				testName, stepIdx, status.FinalizedRoot, expected)
		}
	}
	if checks.LatestFinalizedRootLabel != nil {
		label := *checks.LatestFinalizedRootLabel
		labelRoot := status.FinalizedRoot
		if checks.LatestFinalizedRoot != nil {
			labelRoot = [32]byte(*checks.LatestFinalizedRoot)
		}
		existingRoot, exists := blockRegistry[label]
		if !exists {
			blockRegistry[label] = labelRoot
		} else if existingRoot != labelRoot {
			t.Errorf("[%s] step %d: latestFinalizedRootLabel %q remapped: got %x, want %x",
				testName, stepIdx, label, labelRoot, existingRoot)
		}
		if status.FinalizedRoot != blockRegistry[label] {
			t.Errorf("[%s] step %d: latestFinalizedRootLabel %q mismatch: got %x, want %x",
				testName, stepIdx, label, status.FinalizedRoot, blockRegistry[label])
		}
	}

	if len(checks.AttestationChecks) > 0 {
		for _, ac := range checks.AttestationChecks {
			var sa *types.SignedAttestation
			var found bool
			var locationName string
			if ac.Location == "known" {
				sa, found = store.GetKnownAttestation(ac.Validator)
				locationName = "latest_known_attestations"
			} else {
				sa, found = store.GetNewAttestation(ac.Validator)
				locationName = "latest_new_attestations"
			}

			if !found {
				t.Errorf("[%s] step %d: validator %d not found in %s",
					testName, stepIdx, ac.Validator, locationName)
				continue
			}

			if ac.AttestationSlot != nil && sa.Message.Data.Slot != *ac.AttestationSlot {
				t.Errorf("[%s] step %d: validator %d %s attestation slot: got %d, want %d",
					testName, stepIdx, ac.Validator, locationName, sa.Message.Data.Slot, *ac.AttestationSlot)
			}
			if ac.HeadSlot != nil && sa.Message.Data.Head.Slot != *ac.HeadSlot {
				t.Errorf("[%s] step %d: validator %d %s head slot: got %d, want %d",
					testName, stepIdx, ac.Validator, locationName, sa.Message.Data.Head.Slot, *ac.HeadSlot)
			}
			if ac.SourceSlot != nil && sa.Message.Data.Source.Slot != *ac.SourceSlot {
				t.Errorf("[%s] step %d: validator %d %s source slot: got %d, want %d",
					testName, stepIdx, ac.Validator, locationName, sa.Message.Data.Source.Slot, *ac.SourceSlot)
			}
			if ac.TargetSlot != nil && sa.Message.Data.Target.Slot != *ac.TargetSlot {
				t.Errorf("[%s] step %d: validator %d %s target slot: got %d, want %d",
					testName, stepIdx, ac.Validator, locationName, sa.Message.Data.Target.Slot, *ac.TargetSlot)
			}
		}
	}

	if len(checks.LexicographicHeadAmong) > 0 {
		validateLexicographicHead(t, testName, stepIdx, store, checks.LexicographicHeadAmong, blockRegistry, currentBlockRoot)
	}
}

func validateLexicographicHead(
	t *testing.T,
	testName string,
	stepIdx int,
	store *forkchoice.Store,
	labels []string,
	blockRegistry map[string][32]byte,
	currentBlockRoot *[32]byte,
) {
	t.Helper()

	headRoot := store.GetStatus().Head

	missing := make([]string, 0, len(labels))
	for _, label := range labels {
		if _, ok := blockRegistry[label]; !ok {
			missing = append(missing, label)
		}
	}
	if len(missing) > 0 {
		// Devnet-1 fixtures may omit one competing fork label and imply it is
		// the block introduced by the current step.
		if currentBlockRoot != nil && len(missing) == 1 {
			blockRegistry[missing[0]] = *currentBlockRoot
		} else {
			t.Fatalf("[%s] step %d: unresolved lexicographic labels %v", testName, stepIdx, missing)
		}
	}

	resolved := make([][32]byte, 0, len(labels))
	for _, label := range labels {
		resolved = append(resolved, blockRegistry[label])
	}

	highestRoot := resolved[0]
	for _, root := range resolved[1:] {
		if hashGreater(root, highestRoot) {
			highestRoot = root
		}
	}

	if headRoot != highestRoot {
		t.Errorf("[%s] step %d: lexicographic tiebreaker failed for labels %v: head=%x, expected highest=%x",
			testName, stepIdx, labels, headRoot, highestRoot)
	}
}

// hashGreater returns true if a > b lexicographically.
func hashGreater(a, b [32]byte) bool {
	for i := 0; i < 32; i++ {
		if a[i] > b[i] {
			return true
		}
		if a[i] < b[i] {
			return false
		}
	}
	return false
}
