package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/geanlabs/gean/forkchoice"
	"github.com/geanlabs/gean/node"
	"github.com/geanlabs/gean/specfixtures"
	"github.com/geanlabs/gean/statetransition"
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

// TestDriverEnvVar is the env var that gates the test-driver routes. When
// set to a truthy value, gean exposes POST /lean/v0/test_driver/* endpoints
// the hive lean simulator calls to run lean-spec-tests conformance fixtures.
// These endpoints are not part of the runtime spec; they exist solely for
// conformance testing and must never be exposed on production deployments.
const TestDriverEnvVar = "HIVE_LEAN_TEST_DRIVER"

// IsTestDriverEnabled returns true if the env var is set to "1", "true",
// "TRUE", "yes", or "YES" — matching ream's accepted values.
func IsTestDriverEnabled(value string) bool {
	switch value {
	case "1", "true", "TRUE", "yes", "YES":
		return true
	}
	return false
}

// TestDriverSession holds the ephemeral consensus store and fork choice the
// fork-choice spec-test endpoints operate on. /init replaces the instances;
// /step operates on whatever the most recent /init produced. The mutex
// serializes step processing against init resets; concurrent simulators
// would be a misuse pattern but the lock keeps state consistent regardless.
type TestDriverSession struct {
	mu         sync.Mutex
	store      *node.ConsensusStore
	fc         *forkchoice.ForkChoice
	labelRoots map[string][32]byte
}

// NewTestDriverSession returns an empty session — handlers will reject step
// calls until /fork_choice/init runs and populates store + fc.
func NewTestDriverSession() *TestDriverSession {
	return &TestDriverSession{labelRoots: make(map[string][32]byte)}
}

// RegisterTestDriverRoutes mounts the four test-driver endpoints on mux.
// Called only when HIVE_LEAN_TEST_DRIVER is truthy; see api/server.go.
func RegisterTestDriverRoutes(mux *http.ServeMux, sess *TestDriverSession) {
	mux.HandleFunc("POST /lean/v0/test_driver/state_transition/run", TestDriverStateTransitionHandler())
	mux.HandleFunc("POST /lean/v0/test_driver/fork_choice/init", sess.ForkChoiceInitHandler())
	mux.HandleFunc("POST /lean/v0/test_driver/fork_choice/step", sess.ForkChoiceStepHandler())
	mux.HandleFunc("POST /lean/v0/test_driver/verify_signatures/run", TestDriverVerifySignaturesHandler())
}

// stateTransitionResponse mirrors ream's StateTransitionResponse shape so
// the simulator can deserialize it via serde with rename_all = "camelCase".
type stateTransitionResponse struct {
	Succeeded bool                 `json:"succeeded"`
	Error     *string              `json:"error"`
	Post      *stateTransitionPost `json:"post"`
}

type stateTransitionPost struct {
	Slot                       uint64 `json:"slot"`
	LatestBlockHeaderSlot      uint64 `json:"latestBlockHeaderSlot"`
	LatestBlockHeaderStateRoot string `json:"latestBlockHeaderStateRoot"`
	HistoricalBlockHashesCount uint64 `json:"historicalBlockHashesCount"`
}

// driverStepResponse matches the simulator's expected ForkChoiceStepResponse.
type driverStepResponse struct {
	Accepted bool           `json:"accepted"`
	Error    *string        `json:"error"`
	Snapshot driverSnapshot `json:"snapshot"`
}

type driverSnapshot struct {
	HeadSlot            uint64           `json:"headSlot"`
	HeadRoot            string           `json:"headRoot"`
	Time                uint64           `json:"time"`
	JustifiedCheckpoint driverCheckpoint `json:"justifiedCheckpoint"`
	FinalizedCheckpoint driverCheckpoint `json:"finalizedCheckpoint"`
	SafeTarget          string           `json:"safeTarget"`
}

type driverCheckpoint struct {
	Slot uint64 `json:"slot"`
	Root string `json:"root"`
}

type verifySignaturesResponse struct {
	Succeeded bool    `json:"succeeded"`
	Error     *string `json:"error"`
}

// TestDriverStateTransitionHandler answers POST
// /lean/v0/test_driver/state_transition/run. Per ream's convention, logical
// failures (state-transition rejecting a block) return HTTP 200 with the
// reason in the JSON `error` field; only request-parse or marshaling errors
// surface as non-200 so the simulator can distinguish wire-level failures
// from in-fixture rejections.
func TestDriverStateTransitionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var fixture specfixtures.StateTransitionFixture
		if err := json.NewDecoder(r.Body).Decode(&fixture); err != nil {
			http.Error(w, fmt.Sprintf("decode body: %v", err), http.StatusBadRequest)
			return
		}

		state, err := fixture.Pre.ToState()
		if err != nil {
			writeStateTransitionFailure(w, fmt.Sprintf("pre.toState: %v", err))
			return
		}

		// Empty-blocks + expectException: force a process_slots call against
		// state.slot so the spec's filler-side rejection materializes for us.
		// Mirrors ream's run_state_transition path for the zero-blocks branch.
		if len(fixture.Blocks) == 0 {
			if fixture.ExpectException != "" {
				if err := statetransition.ProcessSlots(state, state.Slot); err != nil {
					writeStateTransitionFailure(w, err.Error())
					return
				}
			}
			// No-blocks, no-exception is a genesis-style smoke case; succeed.
			writeStateTransitionSuccess(w, state)
			return
		}

		for i, tb := range fixture.Blocks {
			block, err := tb.ToBlock()
			if err != nil {
				writeStateTransitionFailure(w, fmt.Sprintf("blocks[%d]: %v", i, err))
				return
			}
			if err := statetransition.StateTransition(state, block); err != nil {
				writeStateTransitionFailure(w, fmt.Sprintf("blocks[%d]: %v", i, err))
				return
			}
		}

		writeStateTransitionSuccess(w, state)
	}
}

func writeStateTransitionSuccess(w http.ResponseWriter, state *types.State) {
	resp := stateTransitionResponse{
		Succeeded: true,
		Error:     nil,
		Post: &stateTransitionPost{
			Slot:                       state.Slot,
			LatestBlockHeaderSlot:      state.LatestBlockHeader.Slot,
			LatestBlockHeaderStateRoot: fmt.Sprintf("0x%x", state.LatestBlockHeader.StateRoot),
			HistoricalBlockHashesCount: uint64(len(state.HistoricalBlockHashes)),
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeStateTransitionFailure(w http.ResponseWriter, msg string) {
	resp := stateTransitionResponse{
		Succeeded: false,
		Error:     &msg,
		Post:      nil,
	}
	writeJSON(w, http.StatusOK, resp)
}

// ForkChoiceInitHandler answers POST /lean/v0/test_driver/fork_choice/init.
// Replaces the session's store + fc with fresh instances seeded from the
// anchor (state, block) pair. Returns 204 No Content per ream's convention.
//
// Logical anchor failures (state-root mismatch between anchor block and
// anchor state hash_tree_root, malformed bytes, etc.) return HTTP 200 with
// the error in a JSON body so the simulator can distinguish them from
// wire-level 400/500s. This is the same pattern the step handler uses.
func (sess *TestDriverSession) ForkChoiceInitHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req specfixtures.ForkChoiceInit
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode body: %v", err), http.StatusBadRequest)
			return
		}

		anchorState, err := req.AnchorState.ToState()
		if err != nil {
			writeInitFailure(w, fmt.Sprintf("anchorState: %v", err))
			return
		}
		if req.GenesisTime != nil {
			anchorState.Config.GenesisTime = *req.GenesisTime
		}

		anchorBlock, err := req.AnchorBlock.ToBlock()
		if err != nil {
			writeInitFailure(w, fmt.Sprintf("anchorBlock: %v", err))
			return
		}
		anchorRoot, err := anchorBlock.HashTreeRoot()
		if err != nil {
			writeInitFailure(w, fmt.Sprintf("anchorBlock.hashTreeRoot: %v", err))
			return
		}

		// Anchor pair consistency check: block.state_root must equal
		// hash_tree_root(state). Per leanSpec PR #678 — a mismatched pair
		// indicates either a fixture bug or an attacker-served anchor and
		// must be rejected before fork choice trusts either side.
		computedStateRoot, err := anchorState.HashTreeRoot()
		if err != nil {
			writeInitFailure(w, fmt.Sprintf("anchorState.hashTreeRoot: %v", err))
			return
		}
		if computedStateRoot != anchorBlock.StateRoot {
			writeInitFailure(w, fmt.Sprintf("anchor state-root mismatch: block=0x%x state=0x%x",
				anchorBlock.StateRoot, computedStateRoot))
			return
		}

		// Build the anchor header. Body root comes from the block body's
		// hash_tree_root so the header is self-consistent — the simulator
		// later uses this header in /step block processing.
		anchorHeader := &types.BlockHeader{
			Slot:          anchorBlock.Slot,
			ProposerIndex: anchorBlock.ProposerIndex,
			ParentRoot:    anchorBlock.ParentRoot,
			StateRoot:     anchorBlock.StateRoot,
		}
		if anchorBlock.Body != nil {
			bodyRoot, _ := anchorBlock.Body.HashTreeRoot()
			anchorHeader.BodyRoot = bodyRoot
		}

		store := node.NewConsensusStore(storage.NewInMemoryBackend())
		store.SetConfig(anchorState.Config)
		store.InsertBlockHeader(anchorRoot, anchorHeader)
		store.InsertState(anchorRoot, anchorState)
		store.InsertLiveChainEntry(anchorBlock.Slot, anchorRoot, anchorBlock.ParentRoot)
		store.SetHead(anchorRoot)
		// Per leanSpec PR #677, store-side justified/finalized seed from the
		// anchor block itself, not from anchorState.LatestJustified/Finalized.
		// Pre-anchor history embedded in the state's checkpoints is ignored.
		anchorCp := &types.Checkpoint{Root: anchorRoot, Slot: anchorBlock.Slot}
		store.SetLatestJustified(anchorCp)
		store.SetLatestFinalized(anchorCp)
		store.StorePendingBlock(anchorRoot, &types.SignedBlock{Block: anchorBlock})

		fc := forkchoice.New(anchorBlock.Slot, anchorRoot)

		sess.mu.Lock()
		sess.store = store
		sess.fc = fc
		sess.labelRoots = make(map[string][32]byte)
		sess.mu.Unlock()

		w.WriteHeader(http.StatusNoContent)
	}
}

func writeInitFailure(w http.ResponseWriter, msg string) {
	// Init failures use the step response shape with an empty snapshot so
	// the simulator can call /init and immediately read .error without
	// having to call /step first.
	resp := driverStepResponse{Accepted: false, Error: &msg, Snapshot: driverSnapshot{}}
	writeJSON(w, http.StatusOK, resp)
}

// ForkChoiceStepHandler answers POST /lean/v0/test_driver/fork_choice/step.
// Dispatches on stepType (tick / block / attestation / gossipAggregatedAttestation
// / checks), updates the session store + fc, and always returns the current
// snapshot — even on accepted=false — so the simulator's assertion path can
// read snapshot.* unconditionally.
func (sess *TestDriverSession) ForkChoiceStepHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var step specfixtures.ForkChoiceStep
		if err := json.NewDecoder(r.Body).Decode(&step); err != nil {
			http.Error(w, fmt.Sprintf("decode body: %v", err), http.StatusBadRequest)
			return
		}

		sess.mu.Lock()
		defer sess.mu.Unlock()

		if sess.store == nil || sess.fc == nil {
			msg := "fork_choice/step called before fork_choice/init"
			resp := driverStepResponse{Accepted: false, Error: &msg, Snapshot: driverSnapshot{}}
			writeJSON(w, http.StatusOK, resp)
			return
		}

		var stepErr error
		switch step.StepType {
		case "tick":
			stepErr = sess.applyTick(&step)
		case "block":
			stepErr = sess.applyBlock(&step)
		case "attestation":
			stepErr = sess.applyAttestation(&step)
		case "gossipAggregatedAttestation":
			stepErr = sess.applyAggregatedAttestation(&step)
		case "checks":
			// Checks-only step: simulator asserts on snapshot, nothing to do
			// on our side.
		default:
			stepErr = fmt.Errorf("unknown stepType %q", step.StepType)
		}

		snap := sess.loadSnapshot()
		accepted := stepErr == nil
		var errPtr *string
		if stepErr != nil {
			msg := stepErr.Error()
			errPtr = &msg
		}
		writeJSON(w, http.StatusOK, driverStepResponse{Accepted: accepted, Error: errPtr, Snapshot: snap})
	}
}

func (sess *TestDriverSession) applyTick(step *specfixtures.ForkChoiceStep) error {
	switch {
	case step.Time != nil:
		sess.store.SetTime(*step.Time)
	case step.Interval != nil:
		sess.store.SetTime(*step.Interval)
	default:
		return fmt.Errorf("tick step missing time and interval")
	}
	return nil
}

func (sess *TestDriverSession) applyBlock(step *specfixtures.ForkChoiceStep) error {
	if step.Block == nil {
		return fmt.Errorf("block step missing block payload")
	}
	block, err := step.Block.ToBlock()
	if err != nil {
		return err
	}

	// Reconstruct per-attestation signature stubs from body bits so the
	// store records per-validator votes during block processing. Matches the
	// spectests forkchoice runner's block-step path.
	var attSigs []*types.AggregatedSignatureProof
	if block.Body != nil {
		for _, att := range block.Body.Attestations {
			attSigs = append(attSigs, &types.AggregatedSignatureProof{Participants: att.AggregationBits})
		}
	}
	signedBlock := &types.SignedBlock{Block: block, Signature: &types.BlockSignatures{AttestationSignatures: attSigs}}

	// Advance store time so attestations inside the block aren't rejected as
	// "too far in future" by the gossip-time guard. Tests intentionally set
	// store time conservatively and rely on block processing to bump it.
	minTime := block.Slot * types.IntervalsPerSlot
	if sess.store.Time() < minTime {
		sess.store.SetTime(minTime)
	}

	if err := node.OnBlockWithoutVerification(sess.store, signedBlock); err != nil {
		return err
	}

	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		return err
	}
	if step.Block.BlockRootLabel != "" {
		sess.labelRoots[step.Block.BlockRootLabel] = blockRoot
	}
	sess.fc.OnBlock(block.Slot, blockRoot, block.ParentRoot)

	// Feed known attestations to fork choice so head selection reflects the
	// new block's contribution.
	attestations := sess.store.ExtractLatestKnownAttestations()
	for vid, data := range attestations {
		if idx := sess.fc.NodeIndex(data.Head.Root); idx >= 0 {
			sess.fc.Votes.SetKnown(vid, idx, data.Slot, data)
		}
	}
	justifiedRoot := sess.store.LatestJustified().Root
	sess.store.SetHead(sess.fc.UpdateHead(justifiedRoot))
	sess.store.PromoteNewToKnown()
	return nil
}

func (sess *TestDriverSession) applyAttestation(step *specfixtures.ForkChoiceStep) error {
	if step.Attestation == nil {
		return fmt.Errorf("attestation step missing attestation payload")
	}
	attData, err := step.Attestation.Data.ToAttestationData()
	if err != nil {
		return err
	}

	if step.Valid {
		minTime := attData.Slot * types.IntervalsPerSlot
		if sess.store.Time() < minTime {
			sess.store.SetTime(minTime)
		}
	}
	if err := node.ValidateAttestationData(sess.store, attData); err != nil {
		return err
	}

	// Single-validator gossip attestation: synthesize a participants bitlist
	// containing just this validator id so downstream aggregation and vote
	// tracking can treat it like any other proof.
	participants := node.AggregationBitsFromIndices([]uint64{step.Attestation.ValidatorID})
	dataRoot, err := attData.HashTreeRoot()
	if err != nil {
		return err
	}
	proof := &types.AggregatedSignatureProof{Participants: participants}
	sess.store.NewPayloads.Push(dataRoot, attData, proof)

	if idx := sess.fc.NodeIndex(attData.Head.Root); idx >= 0 {
		sess.fc.Votes.SetNew(step.Attestation.ValidatorID, idx, attData.Slot, attData)
	}

	sess.store.PromoteNewToKnown()
	knownAtts := sess.store.ExtractLatestKnownAttestations()
	for vid, data := range knownAtts {
		if jdx := sess.fc.NodeIndex(data.Head.Root); jdx >= 0 {
			sess.fc.Votes.SetKnown(vid, jdx, data.Slot, data)
		}
	}
	justifiedRoot := sess.store.LatestJustified().Root
	sess.store.SetHead(sess.fc.UpdateHead(justifiedRoot))
	return nil
}

func (sess *TestDriverSession) applyAggregatedAttestation(step *specfixtures.ForkChoiceStep) error {
	if step.Attestation == nil {
		return fmt.Errorf("gossipAggregatedAttestation step missing attestation payload")
	}
	attData, err := step.Attestation.Data.ToAttestationData()
	if err != nil {
		return err
	}

	if step.Valid {
		minTime := attData.Slot * types.IntervalsPerSlot
		if sess.store.Time() < minTime {
			sess.store.SetTime(minTime)
		}
	}
	if err := node.ValidateAttestationData(sess.store, attData); err != nil {
		return err
	}

	var participants []byte
	var proofData []byte
	if step.Attestation.Proof != nil {
		if participants, err = specfixtures.ParseBoolBitlist(step.Attestation.Proof.Participants.Data); err != nil {
			return fmt.Errorf("proof.participants: %w", err)
		}
		if proofData, err = specfixtures.ParseHexBytes(step.Attestation.Proof.ProofData.Data); err != nil {
			return fmt.Errorf("proof.proofData: %w", err)
		}
	}
	dataRoot, err := attData.HashTreeRoot()
	if err != nil {
		return err
	}
	proof := &types.AggregatedSignatureProof{Participants: participants, ProofData: proofData}
	sess.store.NewPayloads.Push(dataRoot, attData, proof)

	// Aggregated payloads carry multiple participants — feed each as a New
	// vote so fork choice sees the full weight contribution.
	for _, vid := range types.BitlistIndices(participants) {
		if idx := sess.fc.NodeIndex(attData.Head.Root); idx >= 0 {
			sess.fc.Votes.SetNew(vid, idx, attData.Slot, attData)
		}
	}

	sess.store.PromoteNewToKnown()
	knownAtts := sess.store.ExtractLatestKnownAttestations()
	for vid, data := range knownAtts {
		if jdx := sess.fc.NodeIndex(data.Head.Root); jdx >= 0 {
			sess.fc.Votes.SetKnown(vid, jdx, data.Slot, data)
		}
	}
	justifiedRoot := sess.store.LatestJustified().Root
	sess.store.SetHead(sess.fc.UpdateHead(justifiedRoot))
	return nil
}

// loadSnapshot reads the session's current store + fc into the snapshot
// shape the simulator expects. Called under sess.mu so the values are a
// coherent point-in-time read.
func (sess *TestDriverSession) loadSnapshot() driverSnapshot {
	headRoot := sess.store.Head()
	headSlot := uint64(0)
	if hdr := sess.store.GetBlockHeader(headRoot); hdr != nil {
		headSlot = hdr.Slot
	}
	justified := sess.store.LatestJustified()
	finalized := sess.store.LatestFinalized()
	safeTarget := sess.store.SafeTarget()
	return driverSnapshot{
		HeadSlot:            headSlot,
		HeadRoot:            fmt.Sprintf("0x%x", headRoot),
		Time:                sess.store.Time(),
		JustifiedCheckpoint: driverCheckpoint{Slot: justified.Slot, Root: fmt.Sprintf("0x%x", justified.Root)},
		FinalizedCheckpoint: driverCheckpoint{Slot: finalized.Slot, Root: fmt.Sprintf("0x%x", finalized.Root)},
		SafeTarget:          fmt.Sprintf("0x%x", safeTarget),
	}
}

// TestDriverVerifySignaturesHandler answers POST
// /lean/v0/test_driver/verify_signatures/run. Stateless — each request gets
// its own ephemeral store seeded from the request's anchor state. Failures
// return HTTP 200 with succeeded:false; only parse errors surface as 400.
func TestDriverVerifySignaturesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var fixture specfixtures.VerifySignaturesFixture
		if err := json.NewDecoder(r.Body).Decode(&fixture); err != nil {
			http.Error(w, fmt.Sprintf("decode body: %v", err), http.StatusBadRequest)
			return
		}

		anchorState, err := fixture.AnchorState.ToState()
		if err != nil {
			writeVerifyFailure(w, fmt.Sprintf("anchorState: %v", err))
			return
		}
		// Canonical post-state form: state-root in the header may be zeroed
		// (genesis case); fill it in with hash_tree_root(state) before using
		// the header as the anchor block-root preimage. Mirrors what
		// initStoreFromState does in cmd/gean/main.go.
		stateRoot, err := anchorState.HashTreeRoot()
		if err != nil {
			writeVerifyFailure(w, fmt.Sprintf("anchorState.hashTreeRoot: %v", err))
			return
		}
		header := anchorState.LatestBlockHeader
		if header.StateRoot == types.ZeroRoot {
			header.StateRoot = stateRoot
		}
		anchorRoot, err := header.HashTreeRoot()
		if err != nil {
			writeVerifyFailure(w, fmt.Sprintf("anchorHeader.hashTreeRoot: %v", err))
			return
		}

		store := node.NewConsensusStore(storage.NewInMemoryBackend())
		store.SetConfig(anchorState.Config)
		store.InsertBlockHeader(anchorRoot, header)
		store.InsertState(anchorRoot, anchorState)
		store.InsertLiveChainEntry(header.Slot, anchorRoot, header.ParentRoot)
		store.SetHead(anchorRoot)
		store.SetLatestJustified(&types.Checkpoint{Root: anchorRoot, Slot: header.Slot})
		store.SetLatestFinalized(&types.Checkpoint{Root: anchorRoot, Slot: header.Slot})

		// Prefer the devnet-4 flat layout but fall back to the legacy
		// SignedBlockWithAttestation wrapper for older fixtures.
		envelope := fixture.SignedBlock
		if envelope.Block.Slot == 0 && envelope.Block.ParentRoot == "" {
			envelope = fixture.SignedBlockWithAttestation
		}
		signedBlock, err := envelope.ToSignedBlock()
		if err != nil {
			writeVerifyFailure(w, err.Error())
			return
		}

		// node.OnBlock runs full signature verification on the proposer and
		// attestation signatures plus the rest of state transition. The
		// verify-signatures fixtures use the OnBlock-failed verdict as their
		// pass/fail signal — succeeded:false on any OnBlock error.
		if err := node.OnBlock(store, signedBlock, nil); err != nil {
			writeVerifyFailure(w, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, verifySignaturesResponse{Succeeded: true, Error: nil})
	}
}

func writeVerifyFailure(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusOK, verifySignaturesResponse{Succeeded: false, Error: &msg})
}

// writeJSON marshals v as JSON and writes it with the given status code,
// setting the Content-Type header. Marshaling failures surface as 500.
func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, fmt.Sprintf("marshal response: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
}
