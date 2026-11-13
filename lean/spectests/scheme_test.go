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

// fixtureKeyFile is one of the key pairs leanSpec signs every fixture with. All
// fixtures come from one XMSS scheme, so its public-key size is the fixtures'.
const fixtureKeyFile = "../../leanSpec/fixtures/keys/prod_scheme/0.json"

// fixtureKeySize reports the byte length of the fixtures' XMSS public keys, or
// 0 when the key file is missing or unreadable. With 0 nothing is skipped, so a
// fixture layout without key files fails loudly instead of skipping silently.
var fixtureKeySize = sync.OnceValue(func() int {
	raw, err := os.ReadFile(fixtureKeyFile)
	if err != nil {
		return 0
	}
	var keys struct {
		AttestationKeypair struct {
			PublicKey string `json:"public_key"`
		} `json:"attestation_keypair"`
	}
	if err := json.Unmarshal(raw, &keys); err != nil {
		return 0
	}
	return len(strings.TrimPrefix(keys.AttestationKeypair.PublicKey, "0x")) / 2
})

// skipIfFixtureKeySchemeDiffers skips a test whose fixtures carry XMSS keys or
// signatures when leanSpec generated them with a scheme whose public keys have
// a different size from this build's. Such fixtures cannot be decoded, let
// alone verified. The skip lifts itself once leanSpec publishes fixtures for
// the scheme gean runs.
func skipIfFixtureKeySchemeDiffers(t *testing.T) {
	t.Helper()
	if size := fixtureKeySize(); size != 0 && size != types.PubkeySize {
		t.Skipf("leanSpec fixtures use %d-byte XMSS public keys and this build uses %d-byte keys; "+
			"key-dependent fixtures run again once leanSpec publishes fixtures for this scheme",
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
