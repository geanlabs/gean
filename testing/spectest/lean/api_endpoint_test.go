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
	"github.com/geanlabs/gean/internal/lean/transition"
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
	if fixture.Endpoint == "/metrics" {
		skip(t, skipFollowsEthlambda, "gean serves /metrics on its own port, not the API's")
	}
	// An SSZ body holds the state's keys themselves, which a Poseidon key's
	// stand-in cannot reproduce. Every key file holds the scheme's keys, so the
	// first says whether the rest are Poseidon.
	if fixture.ExpectedContentType == "application/octet-stream" {
		skipPoseidonKeys(t, specKeys(t, fixture.LeanEnv, 0).attestation)
	}

	validators := make([]genesis.GenesisValidatorEntry, fixture.GenesisParams.NumValidators)
	for i := range validators {
		keys := specKeys(t, fixture.LeanEnv, uint64(i))
		validators[i] = genesis.GenesisValidatorEntry{
			AttestationPubkey: stateKeyHex(t, i, keys.attestation),
			ProposalPubkey:    stateKeyHex(t, i, keys.proposal),
		}
	}
	genesisConfig := &genesis.GenesisConfig{GenesisTime: fixture.GenesisParams.GenesisTime, GenesisValidators: validators}
	state, err := genesisConfig.GenesisState()
	if err != nil {
		t.Fatalf("genesis state: %v", err)
	}
	// leanSpec reaches a later anchor with an empty block at every slot.
	for state.Slot < fixture.GenesisParams.AnchorSlot {
		if state, err = transition.ApplyEmptyBlock(state); err != nil {
			t.Fatalf("anchor block after slot %d: %v", state.Slot, err)
		}
	}
	s, anchorRoot := newAnchoredStore(t, state)
	fc := forkchoice.New(state.LatestBlockHeader.Slot, anchorRoot, state.LatestBlockHeader.ParentRoot)

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
		// The stand-in keys hash as leanSpec's do, so state and block roots
		// compare exactly.
		var got, want map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if err := json.Unmarshal(fixture.ExpectedBody, &want); err != nil {
			t.Fatalf("decode expected body: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(want)
			t.Errorf("body:\n got  %s\n want %s", gotJSON, wantJSON)
		}
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

// specKeys reads the public keys from one of the spec's deterministic test
// keys, which the fixture download ships under keys/<leanEnv>_scheme. Each key
// file holds a whole XMSS key pair, tens of megabytes, so every file is read
// once a run.
func specKeys(t *testing.T, leanEnv string, index uint64) specKeyPair {
	t.Helper()
	path := filepath.Join(fixturesDirectory(), "keys", leanEnv+"_scheme", strconv.FormatUint(index, 10)+".json")
	if keys, ok := specKeyFiles[path]; ok {
		return keys
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
	keys := specKeyPair{attestation: "0x" + keyFile.AttestationKeypair.PublicKey, proposal: "0x" + keyFile.ProposalKeypair.PublicKey}
	specKeyFiles[path] = keys
	return keys
}

type specKeyPair struct{ attestation, proposal string }

var specKeyFiles = make(map[string]specKeyPair)

// stateKeyHex is the key a genesis state holds for a spec key, a Poseidon key
// standing in by its hash tree root.
func stateKeyHex(t *testing.T, validator int, publicKey string) string {
	t.Helper()
	key, err := specfixtures.ParseStateKey(publicKey)
	if err != nil {
		t.Fatalf("validator %d key: %v", validator, err)
	}
	return "0x" + hex.EncodeToString(key[:])
}
