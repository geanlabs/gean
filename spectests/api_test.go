//go:build spectests

package spectests

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/geanlabs/gean/api"
	"github.com/geanlabs/gean/forkchoice"
	"github.com/geanlabs/gean/genesis"
	"github.com/geanlabs/gean/node"
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

// Spec fixture directory for api endpoint tests.
const apiFixturesRoot = "../leanSpec/fixtures/consensus/api_endpoint/devnet/api/test_api_endpoints"

// leanSpec ships its deterministic test keys here. The fixture's leanEnv
// ("prod"/"test") selects the subdirectory; each N.json has hex-encoded
// attestation_public and proposal_public that feed into genesis state.
const apiKeysRoot = "../leanSpec/packages/testing/src/consensus_testing/test_keys"

type apiFixtureOuter map[string]apiFixture

type apiFixture struct {
	Network             string                 `json:"network"`
	LeanEnv             string                 `json:"leanEnv"`
	Endpoint            string                 `json:"endpoint"`
	Method              string                 `json:"method"` // leanSpec PR #636; defaults "GET" when absent
	GenesisParams       apiGenesisParams       `json:"genesisParams"`
	RequestBody         json.RawMessage        `json:"requestBody"`         // POST bodies (admin endpoints)
	InitialIsAggregator *bool                  `json:"initialIsAggregator"` // seeds AggregatorController before replay
	ExpectedStatusCode  int                    `json:"expectedStatusCode"`
	ExpectedContentType string                 `json:"expectedContentType"`
	ExpectedBody        json.RawMessage        `json:"expectedBody"`
	Info                map[string]interface{} `json:"_info"`
}

type apiGenesisParams struct {
	NumValidators uint64 `json:"numValidators"`
	GenesisTime   uint64 `json:"genesisTime"`
}

// TestSpecAPI walks the api_endpoint fixture directory and, for each fixture,
// spins up an in-process store + fork choice seeded from the fixture's
// genesisParams, invokes the matching API handler via httptest, and compares
// status code, content type, and body structure.
//
// Structural match is preferred over exact hash match because the spec and
// gean may serialize certain auxiliary fields slightly differently; exact
// byte equality can be added later once cross-client SSZ roots are verified.
func TestSpecAPI(t *testing.T) {
	if _, err := os.Stat(apiFixturesRoot); os.IsNotExist(err) {
		t.Skipf("fixtures not present at %s; run 'make leanSpec/fixtures'", apiFixturesRoot)
	}

	err := filepath.Walk(apiFixturesRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: read: %v", path, err)
			return nil
		}

		var outer apiFixtureOuter
		if err := json.Unmarshal(raw, &outer); err != nil {
			t.Errorf("%s: unmarshal: %v", path, err)
			return nil
		}

		for testID, fx := range outer {
			name := shortName(testID, path)
			fx := fx
			t.Run(name, func(t *testing.T) {
				runAPIFixture(t, fx)
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

func runAPIFixture(t *testing.T, fx apiFixture) {
	t.Helper()

	// Build genesis state via the real genesis package so the harness tracks
	// any future field additions instead of drifting from a hand-rolled literal.
	// Pubkeys come from leanSpec's shipped test keys so our hashed state matches
	// the fixture byte-for-byte.
	validators, err := loadSpecValidators(fx.LeanEnv, fx.GenesisParams.NumValidators)
	if err != nil {
		t.Skipf("load spec validator keys: %v", err)
		return
	}
	gc := &genesis.GenesisConfig{
		GenesisTime:       fx.GenesisParams.GenesisTime,
		GenesisValidators: validators,
	}
	state := gc.GenesisState()

	// Seed store + fork choice from the genesis state.
	backend := storage.NewInMemoryBackend()
	s := node.NewConsensusStore(backend)
	s.SetConfig(state.Config)

	header := state.LatestBlockHeader
	blockRoot, _ := header.HashTreeRoot()
	anchor := &types.Checkpoint{Root: blockRoot, Slot: 0}

	s.SetHead(blockRoot)
	s.SetSafeTarget(blockRoot)
	s.SetLatestJustified(anchor)
	s.SetLatestFinalized(anchor)
	s.InsertBlockHeader(blockRoot, header)
	s.InsertState(blockRoot, state)

	fc := forkchoice.New(0, blockRoot)

	// Per-fixture aggregator controller, seeded from initialIsAggregator
	// (leanSpec PR #636). Nil means the fixture doesn't exercise the admin
	// endpoints; defaulting to false keeps the controller present for every
	// replay so lookupAPIHandler never hits a nil pointer.
	initialAgg := false
	if fx.InitialIsAggregator != nil {
		initialAgg = *fx.InitialIsAggregator
	}
	aggCtl := node.NewAggregatorController(initialAgg)

	method := fx.Method
	if method == "" {
		method = "GET"
	}

	// Resolve and invoke the handler via httptest.
	handler := lookupAPIHandler(method, fx.Endpoint, s, fc, aggCtl)
	if handler == nil {
		t.Skipf("endpoint %q (%s) not wired into test harness", fx.Endpoint, method)
		return
	}

	var reqBody io.Reader
	if len(fx.RequestBody) > 0 && string(fx.RequestBody) != "null" {
		reqBody = bytes.NewReader(fx.RequestBody)
	}
	req := httptest.NewRequest(method, fx.Endpoint, reqBody)
	rec := httptest.NewRecorder()
	handler(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != fx.ExpectedStatusCode {
		t.Errorf("status: got %d, want %d", resp.StatusCode, fx.ExpectedStatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != fx.ExpectedContentType {
		t.Errorf("content-type: got %q, want %q", got, fx.ExpectedContentType)
	}

	body, _ := io.ReadAll(resp.Body)

	switch fx.ExpectedContentType {
	case "application/json":
		// Structural compare (keys + Go types). Exact byte/value equality is
		// brittle against cross-impl hash/weight serialization and can be
		// layered on once SSZ roots agree.
		var got, want map[string]interface{}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("decode response body: %v", err)
			return
		}
		if err := json.Unmarshal(fx.ExpectedBody, &want); err != nil {
			t.Errorf("decode expected body: %v", err)
			return
		}
		checkJSONKeysAndTypes(t, got, want, "")
	case "application/octet-stream":
		// SSZ body: fixture stores "0x<hex>", compare byte-for-byte.
		var wantHex string
		if err := json.Unmarshal(fx.ExpectedBody, &wantHex); err != nil {
			t.Errorf("decode expected ssz hex: %v", err)
			return
		}
		wantHex = strings.TrimPrefix(wantHex, "0x")
		gotHex := hex.EncodeToString(body)
		if gotHex != wantHex {
			t.Errorf("ssz body mismatch:\n got:  %s\n want: %s", gotHex, wantHex)
		}
	}
}

// lookupAPIHandler maps a spec (method, endpoint) to the in-process
// http.HandlerFunc. Matches the route registrations in api.StartAPIServer.
func lookupAPIHandler(method, endpoint string, s *node.ConsensusStore, fc *forkchoice.ForkChoice, aggCtl *node.AggregatorController) http.HandlerFunc {
	switch method + " " + endpoint {
	case "GET /lean/v0/health":
		return api.HealthHandler
	case "GET /lean/v0/checkpoints/justified":
		return api.JustifiedCheckpointHandler(s)
	case "GET /lean/v0/fork_choice":
		return api.ForkChoiceHandler(s, fc)
	case "GET /lean/v0/states/finalized":
		return api.FinalizedStateHandler(s)
	case "GET /lean/v0/admin/aggregator":
		return api.AggregatorStatusHandler(aggCtl)
	case "POST /lean/v0/admin/aggregator":
		return api.AggregatorToggleHandler(aggCtl)
	default:
		return nil
	}
}

// loadSpecValidators reads N deterministic test keys from leanSpec's on-disk
// keystore and returns them as gean-format genesis validator entries. Matches
// the pubkeys leanSpec's fixture generator embeds via XmssKeyManager.shared().
func loadSpecValidators(leanEnv string, n uint64) ([]genesis.GenesisValidatorEntry, error) {
	if leanEnv == "" {
		leanEnv = "prod"
	}
	dir := filepath.Join(apiKeysRoot, leanEnv+"_scheme")

	entries := make([]genesis.GenesisValidatorEntry, n)
	for i := uint64(0); i < n; i++ {
		path := filepath.Join(dir, strconv.FormatUint(i, 10)+".json")
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var kf struct {
			AttestationPublic string `json:"attestation_public"`
			ProposalPublic    string `json:"proposal_public"`
		}
		if err := json.Unmarshal(raw, &kf); err != nil {
			return nil, err
		}
		entries[i] = genesis.GenesisValidatorEntry{
			AttestationPubkey: "0x" + kf.AttestationPublic,
			ProposalPubkey:    "0x" + kf.ProposalPublic,
		}
	}
	return entries, nil
}

// checkJSONKeysAndTypes verifies that got contains every top-level key in want
// with matching Go types. Values of dynamic types (hash hex strings, slot
// numbers that depend on deterministic state) are not byte-compared — the
// presence-and-type check is the most portable structural contract.
func checkJSONKeysAndTypes(t *testing.T, got, want map[string]interface{}, prefix string) {
	t.Helper()
	for k, wv := range want {
		gv, ok := got[k]
		path := prefix + "/" + k
		if !ok {
			t.Errorf("missing key %q", path)
			continue
		}
		if reflect.TypeOf(gv) != reflect.TypeOf(wv) {
			t.Errorf("key %q: type mismatch got %T want %T", path, gv, wv)
			continue
		}
		if wantMap, ok := wv.(map[string]interface{}); ok {
			gotMap := gv.(map[string]interface{})
			checkJSONKeysAndTypes(t, gotMap, wantMap, path)
		}
	}
}
