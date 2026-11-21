//go:build spectests

package lean

import (
	"bytes"
	"testing"

	"github.com/geanlabs/gean/internal/core/storage"
	"github.com/geanlabs/gean/internal/lean/attestation"
	"github.com/geanlabs/gean/internal/lean/forkchoice"
	"github.com/geanlabs/gean/internal/lean/importer"
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

// mockedProofSentinel opens every placeholder aggregation proof the upstream
// filler emits in its default (non-real-crypto) mode, where the recursive SNARK
// merge is skipped for vectors that do not test crypto. The cross-client contract
// is that a proof carrying this prefix must be accepted without cryptographic
// verification; any proof or signature lacking it is verified for real. Individual
// attestation signatures are always real, so this only ever short-circuits
// aggregated proofs. This must never leak into production verification: an attacker
// could otherwise prefix a forged proof to bypass the check.
var mockedProofSentinel = []byte("\x00MOCKED-AGGREGATION-PROOF\x00")

func carriesMockedProof(proof []byte) bool {
	return bytes.HasPrefix(proof, mockedProofSentinel)
}

// earliestAdmissibleInterval is the lowest store time at which a slot-N attestation
// clears ValidateAttestationData's future check: admission allows
// slot*INTERVALS_PER_SLOT <= time + GOSSIP_DISPARITY_INTERVALS, so the vote is
// admissible one disparity interval before its slot starts.
func earliestAdmissibleInterval(slot uint64) uint64 {
	start := slot * types.IntervalsPerSlot
	if start < types.GossipDisparityIntervals {
		return 0
	}
	return start - types.GossipDisparityIntervals
}

// runForkChoice replays a fixture's steps against gean's store and fork
// choice, checking the store after each step that carries checks.
func runForkChoice(t *testing.T, fixture *forkChoiceFixture) {
	anchorState, err := fixture.AnchorState.ToState()
	if err != nil {
		t.Fatalf("anchor state: %v", err)
	}
	anchorBlock, err := fixture.AnchorBlock.ToBlock()
	if err != nil {
		t.Fatalf("anchor block: %v", err)
	}

	// Compute anchor block root.
	anchorRoot, err := anchorBlock.HashTreeRoot()
	if err != nil {
		t.Fatalf("computing anchor block root: %v", err)
	}

	backend := storage.NewInMemoryBackend()
	s := store.NewConsensusStore(backend)

	anchorHeader, err := anchorBlock.Header()
	if err != nil {
		t.Fatalf("anchor header: %v", err)
	}

	// Verify the anchor pair is self-consistent: the anchor block's state_root
	// must match hash_tree_root(anchor_state). Without this check, a malformed
	// fixture (or attacker-served (state, block) pair
	// in production) would be accepted after the runner silently rewrote
	// state.LatestBlockHeader.StateRoot to match. Direct compare; the spec
	// expects the input state to arrive with LatestBlockHeader.StateRoot
	// already in the canonical form that makes the equality hold.
	//
	// Empty-steps fixtures with a mismatched pair are negative tests: the
	// spec expected init to abort here. Treat the rejection as pass.
	computedStateRoot, err := anchorState.HashTreeRoot()
	if err != nil {
		t.Fatalf("hashing anchor state: %v", err)
	}
	if computedStateRoot != anchorBlock.StateRoot {
		if len(fixture.Steps) == 0 {
			t.Logf("anchor init rejected (expected): block=%x state=%x",
				anchorBlock.StateRoot, computedStateRoot)
			return
		}
		t.Fatalf("anchor state-root mismatch: block=%x state=%x",
			anchorBlock.StateRoot, computedStateRoot)
	}

	// The checkpoints start at the anchor block itself, not at
	// anchorState.LatestJustified/LatestFinalized: the store treats the anchor as
	// the new genesis and ignores any pre-anchor history in the state.
	if err := s.PutAnchor(anchorRoot, anchorHeader, anchorState); err != nil {
		t.Fatalf("put anchor: %v", err)
	}

	// Store anchor as signed block.
	anchorSigned := &types.SignedBlock{
		Block: anchorBlock,
		Proof: nil,
	}
	s.StorePendingBlock(anchorRoot, anchorSigned)

	fc := forkchoice.New(anchorBlock.Slot, anchorRoot, anchorBlock.ParentRoot)

	// Label -> root map for resolving blockRootLabel references.
	labelRoots := make(map[string][32]byte)

	for i, step := range fixture.Steps {
		switch step.StepType {
		case "block":
			if step.Block == nil {
				t.Fatalf("step %d: block step without block data", i)
			}

			block, err := step.Block.ToBlock()
			if err != nil {
				t.Fatalf("step %d: %v", i, err)
			}

			// Build signatures with participant bits from attestation aggregation_bits
			// so processBlockAttestations stores correct per-validator votes.
			signedBlock := &types.SignedBlock{
				Block: block,
				Proof: &types.MultiMessageAggregate{},
			}

			// Advance the store clock to this block's slot unless the fixture
			// delivers the block ahead of the clock (tickToSlot=false). The clock
			// gates attestation future-validation, so it must reach the slot
			// before subsequent attestations validate.
			if step.TickToSlot == nil || *step.TickToSlot {
				minTime := block.Slot * types.IntervalsPerSlot
				if s.Time() < minTime {
					s.SetTime(minTime)
				}
			}

			// Process block through store (no signature verification).
			if err := importer.OnBlockWithoutVerification(s, signedBlock); err != nil {
				if step.Valid {
					t.Fatalf("step %d: OnBlockWithoutVerification failed: %v", i, err)
				}
				continue
			}

			// Compute block root and register label.
			blockRoot, _ := block.HashTreeRoot()
			if step.Block.BlockRootLabel != "" {
				labelRoots[step.Block.BlockRootLabel] = blockRoot
			}

			// Register block in fork choice.
			fc.OnBlock(block.Slot, blockRoot, block.ParentRoot)

			// Seed the block's on-chain aggregated attestations into the known
			// pool with their participant sets so the votes carry fork-choice
			// weight. The spec test framework merges a block's aggregated proofs
			// into latest_known_aggregated_payloads when building it; block
			// import alone (data, empty proof set) leaves them weightless, which
			// would mis-resolve weight-driven reorgs. Only participants are read
			// during head computation, so a non-empty placeholder proof suffices.
			for _, att := range block.Body.Attestations {
				if att == nil || att.Data == nil || types.BitlistCount(att.AggregationBits) == 0 {
					continue
				}
				dataRoot, err := att.Data.HashTreeRoot()
				if err != nil {
					continue
				}
				s.KnownPayloads.Push(dataRoot, att.Data, &types.SingleMessageAggregate{
					Participants: att.AggregationBits,
					Proof:        []byte{0x01},
				})
			}

			// Update head: extract known attestations, feed to fork choice, compute head.
			attestations := s.ExtractLatestKnownAttestations()
			justifiedRoot := s.LatestJustified().Root

			for vid, data := range attestations {
				fc.SetKnownVote(vid, data.Head.Root, data.Slot, data)
			}

			simulateUpdateHead(s, fc, justifiedRoot)

			// Promote new payloads to known (so next updateHead sees them).
			s.PromoteNewToKnown()

			// Reflect the just-promoted votes into the fork-choice known tracker,
			// as the Engine's next updateHead re-derives known votes from the
			// promoted pool. Without this, a vote gossiped as "new" is promoted in
			// the payload pool but stays absent from the tracker the checks read.
			for vid, data := range s.ExtractLatestKnownAttestations() {
				fc.SetKnownVote(vid, data.Head.Root, data.Slot, data)
			}

			// Validate checks if present.
			validateChecks(t, i, step.Checks, s, fc, labelRoots)

		case "attestation":
			if step.Attestation == nil {
				t.Fatalf("step %d: attestation step without data", i)
			}
			att := step.Attestation
			attData, err := att.Data.ToAttestationData()
			if err != nil {
				t.Fatalf("step %d: %v", i, err)
			}

			// For valid steps, advance time just enough for the attestation to clear
			// the future-admission check. Stop at the earliest admissible interval
			// (one gossip-disparity interval before the vote's slot start), never at
			// the slot start itself: landing there would coincide with a following
			// tick's target and swallow that tick's promotion. Invalid steps keep the
			// current time so the rejection path still fires.
			if step.Valid {
				if minTime := earliestAdmissibleInterval(attData.Slot); s.Time() < minTime {
					s.SetTime(minTime)
				}
			}

			// Run data/bounds validation regardless of step.Valid so rejection
			// fixtures actually exercise the validator. Individual attestation
			// signatures are always real, so verification always runs here.
			dataRoot, _ := attData.HashTreeRoot()
			signature := mustParse(t, specfixtures.ParseHexBytes, att.Signature)
			var validationErr error
			if validationErr = attestation.ValidateAttestationData(s, attData); validationErr == nil && !carriesMockedProof(signature) {
				validationErr = attestation.VerifyGossipAttestation(s, att.ValidatorID, attData, dataRoot, signature)
			}
			if step.Valid && validationErr != nil {
				t.Fatalf("step %d: expected valid attestation, got error: %v", i, validationErr)
			}
			if !step.Valid && validationErr == nil {
				t.Fatalf("step %d: expected invalid attestation, got accepted", i)
			}
			if !step.Valid {
				// Validation correctly rejected — no state mutation, just
				// assert any checks the fixture carried then continue.
				validateChecks(t, i, step.Checks, s, fc, labelRoots)
				continue
			}

			// Store in new payloads with dummy proof.
			participants := types.BitlistFromIndices([]uint64{att.ValidatorID})
			proof := &types.SingleMessageAggregate{
				Participants: participants,
				Proof:        nil,
			}
			s.NewPayloads.Push(dataRoot, attData, proof)

			// Record the raw per-validator signature, mirroring the spec's
			// attestation_signatures pool that the "signatures" check reads.
			var sig [types.SignatureSize]byte
			copy(sig[:], signature)
			s.AttestationSignatures.Insert(dataRoot, attData, att.ValidatorID, sig)

			// Gossip lands in the new pool only. The head keeps reflecting the
			// known pool until a slot-boundary tick promotes these votes, so
			// recompute from the known pool here without promoting.
			knownAtts := s.ExtractLatestKnownAttestations()
			justifiedRoot := s.LatestJustified().Root
			for vid, data := range knownAtts {
				fc.SetKnownVote(vid, data.Head.Root, data.Slot, data)
			}
			simulateUpdateHead(s, fc, justifiedRoot)

			validateChecks(t, i, step.Checks, s, fc, labelRoots)

		case "gossipAggregatedAttestation":
			if step.Attestation == nil {
				t.Fatalf("step %d: gossipAggregatedAttestation step without data", i)
			}
			att := step.Attestation
			attData, err := att.Data.ToAttestationData()
			if err != nil {
				t.Fatalf("step %d: %v", i, err)
			}

			// Advance only to the earliest admissible interval (see the individual
			// attestation case): stopping short of the vote's slot start keeps a later
			// tick's promotion from being skipped. Invalid steps keep the current time.
			if step.Valid {
				if minTime := earliestAdmissibleInterval(attData.Slot); s.Time() < minTime {
					s.SetTime(minTime)
				}
			}

			var participants []byte
			var proofData []byte
			if att.Proof != nil {
				if participants, err = specfixtures.ParseBoolBitlist(att.Proof.Participants.Data); err != nil {
					t.Fatalf("step %d: participants: %v", i, err)
				}
				proofData = mustParse(t, specfixtures.ParseHexBytes, att.Proof.Proof.Data)
			}

			// Symmetric with the individual-attestation case: data/bounds checks
			// always run; the aggregated-proof crypto check is skipped only when
			// the proof is a mocked placeholder. Real proofs — including the short
			// proofs the registry/empty-participant rejection vectors carry — fall
			// through to full verification.
			dataRoot, _ := attData.HashTreeRoot()
			var validationErr error
			if validationErr = attestation.ValidateAttestationData(s, attData); validationErr == nil && !carriesMockedProof(proofData) {
				validationErr = attestation.VerifyAggregatedGossipAttestation(s, attData, participants, proofData)
			}
			if step.Valid && validationErr != nil {
				t.Fatalf("step %d: expected valid aggregated attestation, got error: %v", i, validationErr)
			}
			if !step.Valid && validationErr == nil {
				t.Fatalf("step %d: expected invalid aggregated attestation, got accepted", i)
			}
			if !step.Valid {
				validateChecks(t, i, step.Checks, s, fc, labelRoots)
				continue
			}

			proof := &types.SingleMessageAggregate{
				Participants: participants,
				Proof:        proofData,
			}
			s.NewPayloads.Push(dataRoot, attData, proof)

			// Gossip lands in the new pool only. The head keeps reflecting the
			// known pool until a slot-boundary tick promotes these votes, so
			// recompute from the known pool here without promoting.
			knownAtts := s.ExtractLatestKnownAttestations()
			justifiedRoot := s.LatestJustified().Root
			for vid, data := range knownAtts {
				fc.SetKnownVote(vid, data.Head.Root, data.Slot, data)
			}
			simulateUpdateHead(s, fc, justifiedRoot)

			validateChecks(t, i, step.Checks, s, fc, labelRoots)

		case "tick":
			// step.Time is wall-clock seconds since the UNIX epoch; step.Interval
			// is a raw interval count. Convert to a target interval count.
			var target uint64
			switch {
			case step.Time != nil:
				target = types.TotalIntervals(s.Config().GenesisTime, *step.Time*1000)
			case step.Interval != nil:
				target = *step.Interval
			default:
				t.Fatalf("step %d: tick step without time or interval", i)
			}

			// Advance one interval at a time, mirroring leanSpec on_tick. Stepping is
			// load-bearing: jumping straight to the target would skip the intervening
			// slot-boundary intervals whose actions promote the new-vote pool into the
			// known pool and recompute the head. Promotion happens at interval 4 always,
			// and at interval 0 when a proposal has landed — the latter only on the final
			// interval, matching the spec's should_signal_proposal gate.
			hasProposal := step.HasProposal != nil && *step.HasProposal
			for s.Time() < target {
				next := s.Time() + 1
				s.SetTime(next)
				interval := next % types.IntervalsPerSlot
				signalProposal := hasProposal && next == target
				if interval == 4 || (interval == 0 && signalProposal) {
					s.PromoteNewToKnown()
				}
				if interval == 0 || interval == 4 {
					knownAtts := s.ExtractLatestKnownAttestations()
					for vid, data := range knownAtts {
						fc.SetKnownVote(vid, data.Head.Root, data.Slot, data)
					}
					simulateUpdateHead(s, fc, s.LatestJustified().Root)
				}
			}
			// A tick to at or behind the clock still pins store.time so a later
			// time check reads the fixture's value.
			if target < s.Time() {
				s.SetTime(target)
			}

			validateChecks(t, i, step.Checks, s, fc, labelRoots)

		default:
			t.Fatalf("step %d: unknown step type %q", i, step.StepType)
		}
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

	// Simulate Engine.updateSafeTarget() before reading SafeTarget. Engine
	// fires this at interval 3; the runner cannot model interval cadence
	// directly, so it runs on demand whenever a safeTarget* check is set.
	if checks.SafeTarget != nil || checks.SafeTargetSlot != nil || checks.SafeTargetRootLabel != nil {
		simulateUpdateSafeTarget(s, fc)
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

// simulateUpdateHead recomputes the head and re-anchors finalization to the head's
// chain, mirroring the spec's update_head. Finalization is derived from the canonical
// head rather than advanced during block import, matching the production node.
func simulateUpdateHead(s *store.ConsensusStore, fc *forkchoice.ForkChoice, justifiedRoot [32]byte) {
	newHead := fc.UpdateHead(justifiedRoot)
	s.SetHead(newHead)
	// Track the canonical head's finalized checkpoint unconditionally, mirroring
	// the spec: a higher-finalized fork that loses head selection must not latch.
	if derived := store.DeriveFinalizedFromHead(s, newHead); derived != nil {
		s.SetLatestFinalized(derived)
	}
}

func simulateUpdateSafeTarget(s *store.ConsensusStore, fc *forkchoice.ForkChoice) {
	headState := s.GetState(s.Head())
	if headState == nil {
		return
	}
	justifiedRoot := s.LatestJustified().Root
	numValidators := uint64(len(headState.Validators))
	safeTarget := fc.UpdateSafeTarget(justifiedRoot, numValidators, s.ExtractLatestNewAttestations())
	s.SetSafeTarget(safeTarget)
}
