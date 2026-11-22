//go:build spectests

package lean

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/geanlabs/gean/internal/lean/specfixtures"
)

// Poseidon XMSS public keys are 52 bytes and its signatures 2,536; leanVM's
// BLAKE2s keys are 32 bytes and its signatures 1,208. The sizes tell the
// schemes apart, so these checks stop skipping once leanSpec ships BLAKE2s
// fixtures.
const poseidonSignatureSize = 2536

// skipPoseidonMaterial skips a case whose JSON holds a hex value the size of a
// Poseidon signature, or, when needsKeyBytes is set, of a Poseidon public key.
// A suite that only hashes keys runs on Poseidon keys: specfixtures stands in
// their hash tree roots.
func skipPoseidonMaterial(t *testing.T, raw []byte, needsKeyBytes bool) {
	t.Helper()
	for rest := raw; ; {
		start := bytes.Index(rest, []byte(`"0x`))
		if start < 0 {
			return
		}
		rest = rest[start+3:]
		digits := 0
		for digits < len(rest) && isHexDigit(rest[digits]) {
			digits++
		}
		if digits < len(rest) && rest[digits] == '"' && (digits == 2*poseidonSignatureSize || needsKeyBytes && digits == 2*specfixtures.PoseidonPublicKeySize) {
			skip(t, skipPoseidonXMSS, fmt.Sprintf("%d-byte key or signature", digits/2))
		}
		rest = rest[digits:]
	}
}

func isHexDigit(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

// skipPoseidonKeys skips the case when any hex public key is a Poseidon key,
// for keys read from files rather than from the case itself.
func skipPoseidonKeys(t *testing.T, hexKeys ...string) {
	t.Helper()
	for _, key := range hexKeys {
		if b, err := specfixtures.ParseHexBytes(key); err == nil && len(b) == specfixtures.PoseidonPublicKeySize {
			skip(t, skipPoseidonXMSS, "52-byte public key")
		}
	}
}

// poseidonStructureTypes are leanSpec's Poseidon XMSS structures themselves.
// gean holds BLAKE2s keys and signatures as opaque bytes and has no
// counterpart to check them against.
var poseidonStructureTypes = map[string]bool{
	"Fp":              true,
	"PublicKey":       true,
	"Signature":       true,
	"HashTreeLayer":   true,
	"HashTreeOpening": true,
}
