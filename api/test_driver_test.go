package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geanlabs/gean/specfixtures"
	"github.com/geanlabs/gean/types"
)

// TestIsTestDriverEnabled pins the set of truthy values gean accepts for
// HIVE_LEAN_TEST_DRIVER. ream uses the same set; mirroring it keeps the
// cohort consistent.
func TestIsTestDriverEnabled(t *testing.T) {
	on := []string{"1", "true", "TRUE", "yes", "YES"}
	for _, v := range on {
		if !IsTestDriverEnabled(v) {
			t.Errorf("expected %q to enable test driver", v)
		}
	}
	off := []string{"", "0", "false", "FALSE", "no", "NO", "off", "1 "}
	for _, v := range off {
		if IsTestDriverEnabled(v) {
			t.Errorf("expected %q to leave test driver disabled", v)
		}
	}
}

// genesisTestState returns a minimal valid genesis test state usable as a
// state-transition pre-state or fork-choice/verify-signatures anchor state.
// The latestBlockHeader.stateRoot is set to zero; tests that need a self
// consistent anchor pair re-stamp it with hash_tree_root(state) before use.
func genesisTestState() map[string]any {
	zero := "0x" + strings.Repeat("00", 32)
	return map[string]any{
		"config":                   map[string]any{"genesisTime": 1000},
		"slot":                     0,
		"latestBlockHeader":        map[string]any{"slot": 0, "proposerIndex": 0, "parentRoot": zero, "stateRoot": zero, "bodyRoot": zero},
		"latestJustified":          map[string]any{"root": zero, "slot": 0},
		"latestFinalized":          map[string]any{"root": zero, "slot": 0},
		"historicalBlockHashes":    map[string]any{"data": []any{}},
		"justifiedSlots":           map[string]any{"data": []any{}},
		"validators":               map[string]any{"data": []any{}},
		"justificationsRoots":      map[string]any{"data": []any{}},
		"justificationsValidators": map[string]any{"data": []any{}},
	}
}

// postJSON POSTs payload to handler, decodes the response body into target if
// non-nil and status < 400, and returns the HTTP status code.
func postJSON(t *testing.T, handler http.HandlerFunc, payload any, target any) int {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler(rec, req)
	resp := rec.Result()
	defer resp.Body.Close()
	if target != nil && resp.StatusCode < 400 && resp.ContentLength != 0 {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			t.Fatalf("decode response: %v (status=%d)", err, resp.StatusCode)
		}
	}
	return resp.StatusCode
}

// canonicalStateRoot returns hash_tree_root of the given pre-state, parsed
// via the same path the production handler uses. The pre-state's header
// stateRoot is left untouched — the lean canonical convention is that the
// anchor state is sent with header.stateRoot in whatever form makes
// hash_tree_root(state) == anchorBlock.stateRoot hold. Tests construct the
// state with header.stateRoot=0 and use this root as the anchor block's
// stateRoot, satisfying the equality without mutating the state.
func canonicalStateRoot(t *testing.T, preState map[string]any) string {
	t.Helper()
	body, err := json.Marshal(preState)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	var ts specfixtures.TestState
	if err := json.Unmarshal(body, &ts); err != nil {
		t.Fatalf("unmarshal TestState: %v", err)
	}
	state, err := ts.ToState()
	if err != nil {
		t.Fatalf("ToState: %v", err)
	}
	root, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash_tree_root: %v", err)
	}
	return fmt.Sprintf("0x%x", root)
}

// TestStateTransitionHandler_GenesisNoBlocks pins the empty-blocks happy
// path: a genesis fixture with no blocks and no exception returns
// succeeded:true and a post summary reflecting the pre-state.
func TestStateTransitionHandler_GenesisNoBlocks(t *testing.T) {
	fixture := map[string]any{
		"network": "lean",
		"leanEnv": "lstar",
		"pre":     genesisTestState(),
		"blocks":  []any{},
		"post":    nil,
	}

	var resp stateTransitionResponse
	status := postJSON(t, TestDriverStateTransitionHandler(), fixture, &resp)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if !resp.Succeeded {
		t.Fatalf("expected succeeded=true, got error=%v", resp.Error)
	}
	if resp.Post == nil {
		t.Fatal("expected post != nil on success")
	}
	if resp.Post.Slot != 0 {
		t.Errorf("post.slot: got %d, want 0", resp.Post.Slot)
	}
	if !strings.HasPrefix(resp.Post.LatestBlockHeaderStateRoot, "0x") {
		t.Errorf("post.latestBlockHeaderStateRoot: missing 0x prefix in %q", resp.Post.LatestBlockHeaderStateRoot)
	}
	if resp.Post.HistoricalBlockHashesCount != 0 {
		t.Errorf("post.historicalBlockHashesCount: got %d, want 0", resp.Post.HistoricalBlockHashesCount)
	}
}

// TestStateTransitionHandler_MalformedBody confirms that a wire-level parse
// error returns 400, distinguishing it from logical 200-with-error responses.
func TestStateTransitionHandler_MalformedBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	TestDriverStateTransitionHandler()(rec, req)
	if rec.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 on malformed body, got %d", rec.Result().StatusCode)
	}
}

// TestForkChoiceInit_RejectsMismatchedAnchor confirms the anchor pair
// consistency check fires when block.state_root doesn't match
// hash_tree_root(state). Returns 200 with accepted:false so the simulator
// reads .error uniformly across init and step failures.
func TestForkChoiceInit_RejectsMismatchedAnchor(t *testing.T) {
	state := genesisTestState()
	anchorBlock := map[string]any{
		"slot":          0,
		"proposerIndex": 0,
		"parentRoot":    "0x" + strings.Repeat("00", 32),
		"stateRoot":     "0x" + strings.Repeat("99", 32), // deliberately wrong
		"body":          map[string]any{"attestations": map[string]any{"data": []any{}}},
	}
	req := map[string]any{"anchorState": state, "anchorBlock": anchorBlock}

	sess := NewTestDriverSession()
	var resp driverStepResponse
	status := postJSON(t, sess.ForkChoiceInitHandler(), req, &resp)

	if status != http.StatusOK {
		t.Fatalf("expected 200 even on logical failure, got %d", status)
	}
	if resp.Accepted {
		t.Fatal("expected accepted=false for state-root mismatch, got true")
	}
	if resp.Error == nil || !strings.Contains(*resp.Error, "state-root mismatch") {
		t.Errorf("expected error mentioning state-root mismatch, got %v", resp.Error)
	}
	if sess.store != nil || sess.fc != nil {
		t.Error("rejected init must not populate session store/fc")
	}
}

// TestForkChoiceStep_BeforeInitErrs guarantees a step call without a prior
// init returns the dedicated error rather than panicking on a nil store.
func TestForkChoiceStep_BeforeInitErrs(t *testing.T) {
	sess := NewTestDriverSession()
	tick := uint64(1)
	step := map[string]any{"stepType": "tick", "interval": tick}

	var resp driverStepResponse
	status := postJSON(t, sess.ForkChoiceStepHandler(), step, &resp)

	if status != http.StatusOK {
		t.Fatalf("expected 200 on uninitialized step, got %d", status)
	}
	if resp.Accepted {
		t.Fatal("expected accepted=false on uninitialized step")
	}
	if resp.Error == nil || !strings.Contains(*resp.Error, "before fork_choice/init") {
		t.Errorf("expected error mentioning init, got %v", resp.Error)
	}
}

// TestForkChoiceInitThenStep exercises the round-trip: init with a
// self-consistent anchor, then a tick step advances time, and the snapshot
// reflects it. Pins the state-machine of the session.
func TestForkChoiceInitThenStep(t *testing.T) {
	preState := genesisTestState()
	stateRootHex := canonicalStateRoot(t, preState)
	anchorBlock := map[string]any{
		"slot":          0,
		"proposerIndex": 0,
		"parentRoot":    "0x" + strings.Repeat("00", 32),
		"stateRoot":     stateRootHex,
		"body":          map[string]any{"attestations": map[string]any{"data": []any{}}},
	}

	sess := NewTestDriverSession()
	body, _ := json.Marshal(map[string]any{"anchorState": preState, "anchorBlock": anchorBlock})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	sess.ForkChoiceInitHandler()(rec, req)

	if rec.Result().StatusCode != http.StatusNoContent {
		var fail driverStepResponse
		_ = json.NewDecoder(rec.Result().Body).Decode(&fail)
		reason := "(no reason)"
		if fail.Error != nil {
			reason = *fail.Error
		}
		t.Fatalf("init: expected 204, got %d (reason: %s)", rec.Result().StatusCode, reason)
	}
	if sess.store == nil || sess.fc == nil {
		t.Fatal("init success must populate session store + fc")
	}

	tickInterval := uint64(50)
	tickStep := map[string]any{"stepType": "tick", "interval": tickInterval}

	var stepResp driverStepResponse
	status := postJSON(t, sess.ForkChoiceStepHandler(), tickStep, &stepResp)

	if status != http.StatusOK {
		t.Fatalf("step: expected 200, got %d", status)
	}
	if !stepResp.Accepted {
		t.Fatalf("step: expected accepted=true, got error=%v", stepResp.Error)
	}
	if stepResp.Snapshot.Time != tickInterval {
		t.Errorf("snapshot.time: got %d, want %d", stepResp.Snapshot.Time, tickInterval)
	}
	if !strings.HasPrefix(stepResp.Snapshot.HeadRoot, "0x") {
		t.Errorf("snapshot.headRoot: missing 0x prefix in %q", stepResp.Snapshot.HeadRoot)
	}
}

// TestVerifySignatures_RejectsMalformedAnchor confirms an unparseable anchor
// state returns succeeded:false with the parse error in body, HTTP 200.
func TestVerifySignatures_RejectsMalformedAnchor(t *testing.T) {
	bad := genesisTestState()
	hdr, _ := bad["latestBlockHeader"].(map[string]any)
	hdr["parentRoot"] = "not-hex"
	bad["latestBlockHeader"] = hdr

	fixture := map[string]any{
		"anchorState": bad,
		"signedBlock": map[string]any{
			"block": map[string]any{
				"slot":          0,
				"proposerIndex": 0,
				"parentRoot":    "0x" + strings.Repeat("00", 32),
				"stateRoot":     "0x" + strings.Repeat("00", 32),
				"body":          map[string]any{"attestations": map[string]any{"data": []any{}}},
			},
			"signature": map[string]any{
				"proposerSignature":     "0x" + strings.Repeat("00", types.SignatureSize),
				"attestationSignatures": map[string]any{"data": []any{}},
			},
		},
	}

	var resp verifySignaturesResponse
	status := postJSON(t, TestDriverVerifySignaturesHandler(), fixture, &resp)

	if status != http.StatusOK {
		t.Fatalf("expected 200 on logical failure, got %d", status)
	}
	if resp.Succeeded {
		t.Fatal("expected succeeded=false for malformed anchor")
	}
	if resp.Error == nil {
		t.Error("expected non-nil error on logical failure")
	}
}
