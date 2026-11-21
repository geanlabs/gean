//go:build spectests

package lean

import (
	"bytes"
	"testing"

	"github.com/geanlabs/gean/internal/lean/api/testdriver"
	"github.com/geanlabs/gean/internal/lean/attestation"
	"github.com/geanlabs/gean/internal/lean/forkchoice"
	"github.com/geanlabs/gean/internal/lean/specfixtures"
	"github.com/geanlabs/gean/internal/lean/store"
	"github.com/geanlabs/gean/internal/lean/types"
)

type forkChoiceFixture struct {
	specfixtures.Header
	AnchorState specfixtures.TestState        `json:"anchorState"`
	AnchorBlock specfixtures.TestBlock        `json:"anchorBlock"`
	Steps       []specfixtures.ForkChoiceStep `json:"steps"`
	// MaxSlot is the highest slot the fixture's XMSS keys cover.
	MaxSlot         uint64                 `json:"maxSlot"`
	RejectionReason specfixtures.Unchecked `json:"rejectionReason"`
}

// runForkChoice replays a fixture's steps through the replay the hive driver
// serves, checking each step's outcome and the store after it.
func runForkChoice(t *testing.T, fixture *forkChoiceFixture) {
	anchorState, err := fixture.AnchorState.ToState()
	if err != nil {
		t.Fatalf("anchor state: %v", err)
	}
	anchorBlock, err := fixture.AnchorBlock.ToBlock()
	if err != nil {
		t.Fatalf("anchor block: %v", err)
	}
	replay, err := testdriver.NewReplay(anchorState, anchorBlock)
	if err != nil {
		// A fixture without steps expects the anchor to be refused.
		if len(fixture.Steps) == 0 {
			return
		}
		t.Fatalf("anchor: %v", err)
	}

	for i := range fixture.Steps {
		step := &fixture.Steps[i]
		err := replay.Step(step)
		switch {
		case step.Valid && err != nil:
			t.Fatalf("step %d (%s): rejected: %v", i, step.StepType, err)
		case !step.Valid && err == nil:
			t.Fatalf("step %d (%s): accepted; want rejection", i, step.StepType)
		}
		validateChecks(t, i, step.Checks, replay.Store, replay.FC, replay.LabelRoots)
	}
}

func validateChecks(t *testing.T, stepIdx int, checks *specfixtures.FCChecks, s *store.ConsensusStore, fc *forkchoice.ForkChoice, labelRoots map[string][32]byte) {
	t.Helper()
	if checks == nil {
		return
	}
	// A label the replay never registered fails the check rather than skipping
	// it, so a renamed label or a block that failed to import cannot pass.
	labelRoot := func(label string) [32]byte {
		t.Helper()
		root, ok := labelRoots[label]
		if !ok {
			t.Fatalf("step %d check: unknown block label %q", stepIdx, label)
		}
		return root
	}

	headRoot := s.Head()

	if checks.HeadSlot != nil {
		headHeader := s.GetBlockHeader(headRoot)
		if headHeader == nil {
			t.Fatalf("step %d check: head block header not found for root 0x%x", stepIdx, headRoot)
		}
		if headHeader.Slot != *checks.HeadSlot {
			t.Fatalf("step %d check: headSlot got %d, want %d", stepIdx, headHeader.Slot, *checks.HeadSlot)
		}
	}

	if checks.HeadRoot != nil {
		wantRoot := mustParse(t, specfixtures.ParseHexRoot, *checks.HeadRoot)
		if headRoot != wantRoot {
			t.Fatalf("step %d check: headRoot got 0x%x, want 0x%x", stepIdx, headRoot, wantRoot)
		}
	}

	if checks.HeadRootLabel != nil {
		if want := labelRoot(*checks.HeadRootLabel); headRoot != want {
			t.Fatalf("step %d check: headRootLabel %q got 0x%x, want 0x%x",
				stepIdx, *checks.HeadRootLabel, headRoot, want)
		}
	}

	if checks.Time != nil {
		if got := s.Time(); got != *checks.Time {
			t.Fatalf("step %d check: time got %d, want %d", stepIdx, got, *checks.Time)
		}
	}

	lj := s.LatestJustified()
	if checks.LatestJustifiedSlot != nil && lj.Slot != *checks.LatestJustifiedSlot {
		t.Fatalf("step %d check: latestJustifiedSlot got %d, want %d",
			stepIdx, lj.Slot, *checks.LatestJustifiedSlot)
	}
	if checks.LatestJustifiedRoot != nil {
		want := mustParse(t, specfixtures.ParseHexRoot, *checks.LatestJustifiedRoot)
		if lj.Root != want {
			t.Fatalf("step %d check: latestJustifiedRoot got 0x%x, want 0x%x",
				stepIdx, lj.Root, want)
		}
	}
	if checks.LatestJustifiedRootLabel != nil {
		if want := labelRoot(*checks.LatestJustifiedRootLabel); lj.Root != want {
			t.Fatalf("step %d check: latestJustifiedRootLabel %q got 0x%x, want 0x%x",
				stepIdx, *checks.LatestJustifiedRootLabel, lj.Root, want)
		}
	}

	lf := s.LatestFinalized()
	if checks.LatestFinalizedSlot != nil && lf.Slot != *checks.LatestFinalizedSlot {
		t.Fatalf("step %d check: latestFinalizedSlot got %d, want %d",
			stepIdx, lf.Slot, *checks.LatestFinalizedSlot)
	}
	if checks.LatestFinalizedRoot != nil {
		want := mustParse(t, specfixtures.ParseHexRoot, *checks.LatestFinalizedRoot)
		if lf.Root != want {
			t.Fatalf("step %d check: latestFinalizedRoot got 0x%x, want 0x%x",
				stepIdx, lf.Root, want)
		}
	}
	if checks.LatestFinalizedRootLabel != nil {
		if want := labelRoot(*checks.LatestFinalizedRootLabel); lf.Root != want {
			t.Fatalf("step %d check: latestFinalizedRootLabel %q got 0x%x, want 0x%x",
				stepIdx, *checks.LatestFinalizedRootLabel, lf.Root, want)
		}
	}

	st := s.SafeTarget()
	if checks.SafeTarget != nil {
		want := mustParse(t, specfixtures.ParseHexRoot, *checks.SafeTarget)
		if st != want {
			t.Fatalf("step %d check: safeTarget got 0x%x, want 0x%x", stepIdx, st, want)
		}
	}
	if checks.SafeTargetRootLabel != nil {
		if want := labelRoot(*checks.SafeTargetRootLabel); st != want {
			t.Fatalf("step %d check: safeTargetRootLabel %q got 0x%x, want 0x%x",
				stepIdx, *checks.SafeTargetRootLabel, st, want)
		}
	}
	if checks.SafeTargetSlot != nil {
		stHeader := s.GetBlockHeader(st)
		if stHeader == nil {
			t.Fatalf("step %d check: safe target header not found for root 0x%x", stepIdx, st)
		}
		if stHeader.Slot != *checks.SafeTargetSlot {
			t.Fatalf("step %d check: safeTargetSlot got %d, want %d",
				stepIdx, stHeader.Slot, *checks.SafeTargetSlot)
		}
	}

	if checks.AttestationTargetSlot != nil {
		target := attestation.GetAttestationTarget(s)
		if target.Slot != *checks.AttestationTargetSlot {
			t.Fatalf("step %d check: attestationTargetSlot got %d, want %d",
				stepIdx, target.Slot, *checks.AttestationTargetSlot)
		}
	}

	if len(checks.LexicographicHeadAmong) > 0 {
		found := false
		for _, label := range checks.LexicographicHeadAmong {
			if root, ok := labelRoots[label]; ok && root == headRoot {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("step %d check: head 0x%x not in lexicographicHeadAmong %v",
				stepIdx, headRoot, checks.LexicographicHeadAmong)
		}
	}

	if len(checks.CanonicalEquivocationHeadAmong) > 0 {
		validateCanonicalEquivocationHead(t, stepIdx, checks.CanonicalEquivocationHeadAmong, s, headRoot, labelRoots)
	}

	for _, ac := range checks.AttestationChecks {
		validateAttestationCheck(t, stepIdx, s, fc, ac)
	}
}

// validateCanonicalEquivocationHead mirrors leanSpec _validate_canonical_equivocation_head:
// the equal-slot equivocation tie breaks toward the fork carrying the largest
// attestation-data root, read from the accepted aggregated pool rather than hardcoded,
// so the assertion is independent of the signature scheme (roots embed validator keys).
func validateCanonicalEquivocationHead(t *testing.T, stepIdx int, forkLabels []string, s *store.ConsensusStore, headRoot [32]byte, labelRoots map[string][32]byte) {
	t.Helper()
	if len(forkLabels) < 2 {
		t.Fatalf("step %d check: canonicalEquivocationHeadAmong needs >=2 forks, got %v", stepIdx, forkLabels)
	}

	forkRoots := make(map[string][32]byte, len(forkLabels))
	for _, label := range forkLabels {
		root, ok := labelRoots[label]
		if !ok {
			t.Fatalf("step %d check: canonicalEquivocationHeadAmong label %q not in block registry", stepIdx, label)
		}
		forkRoots[label] = root
	}

	// Largest attestation-data root per fork — the key ExtractLatestAttestations sorts on.
	// Scan the whole accepted aggregated pool (new + promoted): a tick check may run
	// before the slot-boundary promotion moves votes from the new pool into the known one.
	maxAttRoot := make(map[string][32]byte, len(forkLabels))
	scan := func(entries map[[32]byte]*store.PayloadEntry) {
		for _, entry := range entries {
			if entry.Data == nil || entry.Data.Target == nil {
				continue
			}
			dataRoot, err := entry.Data.HashTreeRoot()
			if err != nil {
				t.Fatalf("step %d check: attestation-data root: %v", stepIdx, err)
			}
			for label, forkRoot := range forkRoots {
				if entry.Data.Target.Root != forkRoot {
					continue
				}
				if cur, seen := maxAttRoot[label]; !seen || bytes.Compare(dataRoot[:], cur[:]) > 0 {
					maxAttRoot[label] = dataRoot
				}
			}
		}
	}
	scan(s.NewPayloads.Entries())
	scan(s.KnownPayloads.Entries())

	var winner string
	var winnerRoot [32]byte
	for _, label := range forkLabels {
		root, ok := maxAttRoot[label]
		if !ok {
			t.Fatalf("step %d check: canonicalEquivocationHeadAmong fork %q has no attestation targeting it in the accepted aggregated pool", stepIdx, label)
		}
		if winner == "" || bytes.Compare(root[:], winnerRoot[:]) > 0 {
			winner, winnerRoot = label, root
		}
	}

	if headRoot != forkRoots[winner] {
		t.Fatalf("step %d check: canonical equivocation tiebreak: head 0x%x, want fork %q 0x%x (largest attestation-data root)",
			stepIdx, headRoot, winner, forkRoots[winner])
	}
}

func validateAttestationCheck(t *testing.T, stepIdx int, s *store.ConsensusStore, fc *forkchoice.ForkChoice, ac specfixtures.FCAttestationCheck) {
	t.Helper()
	var target *forkchoice.VoteTarget
	switch ac.Location {
	case "new", "known":
		tracker, ok := fc.VoteTracker(ac.Validator)
		if !ok {
			t.Fatalf("step %d: attestationCheck v=%d location=%q: no vote tracker", stepIdx, ac.Validator, ac.Location)
		}
		if ac.Location == "new" {
			target = tracker.LatestNew
		} else {
			target = tracker.LatestKnown
		}
	case "signatures":
		target = latestSignatureVote(s, ac.Validator)
	default:
		t.Fatalf("step %d: attestationCheck v=%d: unsupported location %q (want \"new\", \"known\" or \"signatures\")",
			stepIdx, ac.Validator, ac.Location)
	}
	if target == nil {
		t.Fatalf("step %d: attestationCheck v=%d %s: no attestation present in pool",
			stepIdx, ac.Validator, ac.Location)
	}
	if ac.AttestationSlot != nil && target.Slot != *ac.AttestationSlot {
		t.Errorf("step %d: attestationCheck v=%d %s: attestationSlot got %d, want %d",
			stepIdx, ac.Validator, ac.Location, target.Slot, *ac.AttestationSlot)
	}
	if target.Data == nil {
		if ac.HeadSlot != nil || ac.SourceSlot != nil || ac.TargetSlot != nil {
			t.Fatalf("step %d: attestationCheck v=%d %s: tracker has no AttestationData",
				stepIdx, ac.Validator, ac.Location)
		}
		return
	}
	for _, slotCheck := range []struct {
		name       string
		want       *uint64
		checkpoint *types.Checkpoint
	}{
		{"headSlot", ac.HeadSlot, target.Data.Head},
		{"sourceSlot", ac.SourceSlot, target.Data.Source},
		{"targetSlot", ac.TargetSlot, target.Data.Target},
	} {
		switch {
		case slotCheck.want == nil:
		case slotCheck.checkpoint == nil:
			t.Errorf("step %d: attestationCheck v=%d %s: %s wanted but the attestation has no checkpoint",
				stepIdx, ac.Validator, ac.Location, slotCheck.name)
		case slotCheck.checkpoint.Slot != *slotCheck.want:
			t.Errorf("step %d: attestationCheck v=%d %s: %s got %d, want %d",
				stepIdx, ac.Validator, ac.Location, slotCheck.name, slotCheck.checkpoint.Slot, *slotCheck.want)
		}
	}
}

// latestSignatureVote mirrors the spec's "signatures" location: scan the raw
// attestation-signature pool and return the validator's highest-slot vote
// (first seen wins on equal slots, matching the fork-choice rule). Returns nil
// when the validator has no signature recorded.
func latestSignatureVote(s *store.ConsensusStore, validator uint64) *forkchoice.VoteTarget {
	var latest *types.AttestationData
	for _, entry := range s.AttestationSignatures.Snapshot() {
		for _, sig := range entry.Signatures {
			if sig.ValidatorID != validator {
				continue
			}
			if latest == nil || latest.Slot < entry.Data.Slot {
				latest = entry.Data
			}
		}
	}
	if latest == nil {
		return nil
	}
	return &forkchoice.VoteTarget{Slot: latest.Slot, Data: latest}
}
