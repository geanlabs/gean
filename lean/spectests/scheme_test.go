//go:build spectests

package spectests

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/geanlabs/gean/lean/consensus/types"
)

// pinnedFixtureKeySize is the spec tests' compatibility status: the XMSS
// public-key size of the leanSpec fixtures pinned by LEAN_SPEC_FIXTURES_SHA256
// in the Makefile. While it differs from types.PubkeySize, the suites that
// carry XMSS keys or signatures (state transition, fork choice, signatures,
// proofs, sync, API, and the SSZ cases holding keys or signatures) do not run,
// so those rules are not validated against the specification. Update it with
// the pin; the tests fail if it disagrees with the fixtures.
const pinnedFixtureKeySize = 52

// fixtureKeyFile is one of the key pairs leanSpec signs every fixture with. All
// fixtures come from one XMSS scheme, so its public-key size is the fixtures'.
const fixtureKeyFile = "../../leanSpec/fixtures/keys/prod_scheme/0.json"

var fixtureKeySize = sync.OnceValues(func() (int, error) {
	raw, err := os.ReadFile(fixtureKeyFile)
	if err != nil {
		return 0, err
	}
	var keys struct {
		AttestationKeypair struct {
			PublicKey string `json:"public_key"`
		} `json:"attestation_keypair"`
	}
	if err := json.Unmarshal(raw, &keys); err != nil {
		return 0, err
	}
	return len(strings.TrimPrefix(keys.AttestationKeypair.PublicKey, "0x")) / 2, nil
})

// skipIfFixtureKeySchemeDiffers skips a test whose fixtures carry XMSS keys or
// signatures while the pinned fixtures use a key size this build cannot decode,
// and fails if the fixtures do not match the declared compatibility status.
func skipIfFixtureKeySchemeDiffers(t *testing.T) {
	t.Helper()
	size, err := fixtureKeySize()
	if err != nil {
		t.Fatalf("read fixture key size from %s: %v", fixtureKeyFile, err)
	}
	if size != pinnedFixtureKeySize {
		t.Fatalf("fixtures carry %d-byte XMSS public keys but pinnedFixtureKeySize declares %d; update it with the fixture pin",
			size, pinnedFixtureKeySize)
	}
	if size != types.PubkeySize {
		t.Skipf("not validated: the pinned leanSpec fixtures use %d-byte XMSS public keys and this build uses %d-byte keys",
			size, types.PubkeySize)
	}
}

// carriesXMSSMaterial reports whether an SSZ fixture value holds XMSS public
// keys or a signature, whose sizes depend on the scheme.
func carriesXMSSMaterial(value []byte) bool {
	for _, field := range []string{`"attestationPublicKey"`, `"proposalPublicKey"`, `"signature"`} {
		if bytes.Contains(value, []byte(field)) {
			return true
		}
	}
	return false
}
