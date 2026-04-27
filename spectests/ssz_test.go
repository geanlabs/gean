//go:build spectests

package spectests

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geanlabs/gean/types"
)

// Spec fixture roots for SSZ conformance tests. Each directory holds fixtures
// that carry typeName / serialized (SSZ hex) / root (hash_tree_root hex).
var sszFixtureRoots = []string{
	"../leanSpec/fixtures/consensus/ssz/devnet/ssz/test_consensus_containers",
	"../leanSpec/fixtures/consensus/ssz/devnet/ssz/test_networking_containers",
	"../leanSpec/fixtures/consensus/ssz/devnet/ssz/test_xmss_containers",
}

type sszFixtureOuter map[string]sszFixture

type sszFixture struct {
	Network    string          `json:"network"`
	LeanEnv    string          `json:"leanEnv"`
	TypeName   string          `json:"typeName"`
	Value      json.RawMessage `json:"value"`
	Serialized string          `json:"serialized"`
	Root       string          `json:"root"`
}

// sszCodec is the uniform contract every gean SSZ type satisfies (sszgen
// output). The value JSON is intentionally ignored — `serialized` and `root`
// are the canonical spec outputs, and exercising the gean codec against them
// covers the decoder, the encoder (via round-trip), and the hasher.
type sszCodec interface {
	UnmarshalSSZ(buf []byte) error
	MarshalSSZ() ([]byte, error)
	HashTreeRoot() ([32]byte, error)
}

// hashSkipTypes are container types whose hash_tree_root is known to diverge
// between leanSpec (Pydantic Container-merkleize of inner Signature fields)
// and the broader client ecosystem (gean, lean-Rust treat Signature as a
// fixed-bytes blob; ethlambda and zeam don't compute these roots at all).
//
// No client computes hash_tree_root on SignedBlock / BlockSignatures /
// SignedAttestation on the consensus hot path — block roots come from
// hash_tree_root(Block), with the signature outside the root — so the
// divergence isn't a live interop problem. We still exercise encode/decode
// round-trip for these types; only the hash check is skipped. Resolution
// should land upstream in leanSpec (either override hash_tree_root on
// Signature to match the ecosystem, or drop the dead-path fixtures).
var hashSkipTypes = map[string]bool{
	"BlockSignatures":   true,
	"SignedAttestation": true,
	"SignedBlock":       true,
}

// sszFactories maps the spec typeName to a zero-value gean struct. Only types
// whose gean implementation already satisfies sszCodec are registered here;
// fixtures for other types (Signature, PublicKey, HashTreeLayer, HashTreeOpening,
// Status, BlocksByRootRequest) are skipped at runtime with a clear reason.
var sszFactories = map[string]func() sszCodec{
	"Checkpoint":                  func() sszCodec { return new(types.Checkpoint) },
	"Config":                      func() sszCodec { return new(types.ChainConfig) },
	"Validator":                   func() sszCodec { return new(types.Validator) },
	"BlockHeader":                 func() sszCodec { return new(types.BlockHeader) },
	"BlockBody":                   func() sszCodec { return new(types.BlockBody) },
	"Block":                       func() sszCodec { return new(types.Block) },
	"BlockSignatures":             func() sszCodec { return new(types.BlockSignatures) },
	"SignedBlock":                 func() sszCodec { return new(types.SignedBlock) },
	"AttestationData":             func() sszCodec { return new(types.AttestationData) },
	"Attestation":                 func() sszCodec { return new(types.Attestation) },
	"SignedAttestation":           func() sszCodec { return new(types.SignedAttestation) },
	"AggregatedAttestation":       func() sszCodec { return new(types.AggregatedAttestation) },
	"SignedAggregatedAttestation": func() sszCodec { return new(types.SignedAggregatedAttestation) },
	"AggregatedSignatureProof":    func() sszCodec { return new(types.AggregatedSignatureProof) },
	"State":                       func() sszCodec { return new(types.State) },
}

// TestSpecSSZ walks the ssz container fixture directories and, for each
// fixture, exercises the gean codec end-to-end:
//
//  1. hex-decode `serialized` into bytes.
//  2. UnmarshalSSZ(bytes) into a zero-value gean struct — exercises the decoder.
//  3. MarshalSSZ() the struct again and assert byte-for-byte equal to (1) —
//     exercises the encoder via round-trip.
//  4. HashTreeRoot() and assert equal to the fixture's `root` — exercises
//     merkleization independently of encode/decode.
func TestSpecSSZ(t *testing.T) {
	var walked int
	for _, root := range sszFixtureRoots {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			t.Logf("skipping %s: not present", root)
			continue
		}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".json") {
				return err
			}
			walked++

			raw, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("%s: read: %v", path, err)
				return nil
			}

			var outer sszFixtureOuter
			if err := json.Unmarshal(raw, &outer); err != nil {
				t.Errorf("%s: unmarshal: %v", path, err)
				return nil
			}

			for testID, fx := range outer {
				name := shortName(testID, path)
				fx := fx
				t.Run(name, func(t *testing.T) {
					runSSZFixture(t, fx)
				})
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}

	if walked == 0 {
		t.Skip("no ssz fixtures found; run 'make leanSpec/fixtures'")
	}
}

func runSSZFixture(t *testing.T, fx sszFixture) {
	t.Helper()

	factory, ok := sszFactories[fx.TypeName]
	if !ok {
		t.Skipf("type %q not wired into ssz harness", fx.TypeName)
		return
	}

	want, err := hex.DecodeString(strings.TrimPrefix(fx.Serialized, "0x"))
	if err != nil {
		t.Fatalf("decode serialized hex: %v", err)
	}

	// (2) Decode spec-canonical bytes into a gean struct.
	obj := factory()
	if err := obj.UnmarshalSSZ(want); err != nil {
		t.Fatalf("UnmarshalSSZ: %v", err)
	}

	// (3) Re-encode and assert byte-for-byte equality with the input.
	got, err := obj.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("ssz round-trip mismatch:\n got:  %s\n want: %s",
			hex.EncodeToString(got), hex.EncodeToString(want))
	}

	// (4) Independently hash and compare to the fixture's expected root.
	// Skip known spec-ecosystem divergences — see hashSkipTypes for rationale.
	if hashSkipTypes[fx.TypeName] {
		return
	}
	hash, err := obj.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot: %v", err)
	}
	gotRoot := "0x" + hex.EncodeToString(hash[:])
	wantRoot := strings.ToLower(fx.Root)
	if !strings.HasPrefix(wantRoot, "0x") {
		wantRoot = "0x" + wantRoot
	}
	if gotRoot != wantRoot {
		t.Errorf("hash_tree_root mismatch:\n got:  %s\n want: %s", gotRoot, wantRoot)
	}
}
