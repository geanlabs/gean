//go:build spectests

package lean

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/lean/api"
	"github.com/geanlabs/gean/internal/lean/forkchoice"
	"github.com/geanlabs/gean/internal/lean/genesis"
	"github.com/geanlabs/gean/internal/lean/role"
	"github.com/geanlabs/gean/internal/lean/specfixtures"
)

type apiEndpointFixture struct {
	specfixtures.Header
	Endpoint      string `json:"endpoint"`
	Method        string `json:"method"`
	GenesisParams struct {
		NumValidators uint64 `json:"numValidators"`
		GenesisTime   uint64 `json:"genesisTime"`
		// AnchorSlot, when set, starts the node from a later anchor than genesis.
		AnchorSlot uint64 `json:"anchorSlot"`
	} `json:"genesisParams"`
	RequestBody         json.RawMessage `json:"requestBody"`
	InitialIsAggregator bool            `json:"initialIsAggregator"`
	ExpectedStatusCode  int             `json:"expectedStatusCode"`
	ExpectedContentType string          `json:"expectedContentType"`
	ExpectedBody        json.RawMessage `json:"expectedBody"`
}

// runAPIEndpoint serves the fixture's request from a store seeded with the
// fixture's genesis, and compares the status, content type and body.
func runAPIEndpoint(t *testing.T, fixture *apiEndpointFixture) {
	// Every key file holds the scheme's keys, so the first says whether the
	// rest are Poseidon before any more are read.
	skipPoseidonKeys(t, specValidator(t, fixture.LeanEnv, 0).AttestationPubkey)
	if fixture.GenesisParams.AnchorSlot > 0 {
		skip(t, skipNotImplemented, "API fixtures anchored after genesis")
	}

	validators := make([]genesis.GenesisValidatorEntry, fixture.GenesisParams.NumValidators)
	for i := range validators {
		validators[i] = specValidator(t, fixture.LeanEnv, uint64(i))
	}
	genesisConfig := &genesis.GenesisConfig{GenesisTime: fixture.GenesisParams.GenesisTime, GenesisValidators: validators}
	state, err := genesisConfig.GenesisState()
	if err != nil {
		t.Fatalf("genesis state: %v", err)
	}
	s, anchorRoot := newAnchoredStore(t, state)
	fc := forkchoice.New(0, anchorRoot, [32]byte{})

	var requestBody io.Reader
	if len(fixture.RequestBody) > 0 && string(fixture.RequestBody) != "null" {
		requestBody = bytes.NewReader(fixture.RequestBody)
	}
	recorder := httptest.NewRecorder()
	api.Handler(s, fc, role.New(fixture.InitialIsAggregator, nil)).
		ServeHTTP(recorder, httptest.NewRequest(fixture.Method, fixture.Endpoint, requestBody))
	response := recorder.Result()
	defer response.Body.Close()

	if response.StatusCode != fixture.ExpectedStatusCode {
		t.Errorf("status: got %d, want %d", response.StatusCode, fixture.ExpectedStatusCode)
	}
	if got := response.Header.Get("Content-Type"); got != fixture.ExpectedContentType {
		t.Errorf("content type: got %q, want %q", got, fixture.ExpectedContentType)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	switch fixture.ExpectedContentType {
	case "application/json":
		// Keys and JSON types are compared, not values: roots and weights in the
		// body depend on the keys and state the spec used.
		var got, want map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if err := json.Unmarshal(fixture.ExpectedBody, &want); err != nil {
			t.Fatalf("decode expected body: %v", err)
		}
		checkJSONKeysAndTypes(t, got, want, "")
	case "application/octet-stream":
		var wantHex string
		if err := json.Unmarshal(fixture.ExpectedBody, &wantHex); err != nil {
			t.Fatalf("decode expected SSZ hex: %v", err)
		}
		if gotHex := hex.EncodeToString(body); gotHex != strings.TrimPrefix(wantHex, "0x") {
			t.Errorf("SSZ body: got %s, want %s", gotHex, wantHex)
		}
	}
}

// specValidator reads one of the spec's deterministic test keys, which the
// fixture download ships under keys/<leanEnv>_scheme. Each key file holds a
// whole XMSS key pair, tens of megabytes, so every file is read once a run.
func specValidator(t *testing.T, leanEnv string, index uint64) genesis.GenesisValidatorEntry {
	t.Helper()
	path := filepath.Join(fixturesDirectory(), "keys", leanEnv+"_scheme", strconv.FormatUint(index, 10)+".json")
	if validator, ok := specValidators[path]; ok {
		return validator
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("spec validator key: %v", err)
	}
	var keyFile struct {
		AttestationKeypair struct {
			PublicKey string `json:"public_key"`
		} `json:"attestation_keypair"`
		ProposalKeypair struct {
			PublicKey string `json:"public_key"`
		} `json:"proposal_keypair"`
	}
	if err := json.Unmarshal(raw, &keyFile); err != nil {
		t.Fatalf("spec validator key %s: %v", path, err)
	}
	validator := genesis.GenesisValidatorEntry{
		AttestationPubkey: "0x" + keyFile.AttestationKeypair.PublicKey,
		ProposalPubkey:    "0x" + keyFile.ProposalKeypair.PublicKey,
	}
	specValidators[path] = validator
	return validator
}

var specValidators = make(map[string]genesis.GenesisValidatorEntry)

// checkJSONKeysAndTypes checks that got has every key in want, at every depth,
// with the same JSON type.
func checkJSONKeysAndTypes(t *testing.T, got, want map[string]any, prefix string) {
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
		if wantMap, ok := wv.(map[string]any); ok {
			gotMap := gv.(map[string]any)
			checkJSONKeysAndTypes(t, gotMap, wantMap, path)
		}
	}
}
