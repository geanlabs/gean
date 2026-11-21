//go:build !fakecrypto

package xmss

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateSchemeVector = flag.Bool("update", false, "rewrite testdata/scheme_vector.json from this build")

// schemeVector is a key and signature this build of the XMSS library made from
// fixed inputs.
type schemeVector struct {
	PublicKey string `json:"publicKey"`
	Signature string `json:"signature"`
}

// The XMSS library must derive the same key and make the same signature from
// the same inputs as when the vector was recorded, and the signature must
// verify. A leanVM upgrade can change the scheme while every size stays the
// same, as the move from Poseidon to BLAKE2s did, which would leave every
// genesis key and stored proof unusable; this fails on such an upgrade instead
// of a devnet. Generation and signing are deterministic, so the vector is
// exact.
func TestSchemeKnownAnswer(t *testing.T) {
	const slot = 7
	message := [MessageLength]byte{0x67, 0x65, 0x61, 0x6e}
	path := filepath.Join("testdata", "scheme_vector.json")

	kp, err := GenerateKeyPair("gean-scheme-vector", 0, 64)
	if err != nil {
		t.Fatal(err)
	}
	defer kp.Close()
	publicKey, err := kp.PublicKeyBytes()
	if err != nil {
		t.Fatal(err)
	}
	signature, err := kp.Sign(slot, message)
	if err != nil {
		t.Fatal(err)
	}

	got := schemeVector{hex.EncodeToString(publicKey[:]), hex.EncodeToString(signature[:])}

	if *updateSchemeVector {
		data, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var vector schemeVector
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatal(err)
	}
	const changed = "the XMSS scheme changed: keys and signatures from earlier builds no longer match. " +
		"If a leanVM upgrade means to change it, rerun this test with -update"
	if got.PublicKey != vector.PublicKey {
		t.Errorf("%s: derived public key %s, recorded %s", changed, got.PublicKey, vector.PublicKey)
	}
	if got.Signature != vector.Signature {
		t.Errorf("%s: signature differs from the recorded one", changed)
	}
	if ok, err := VerifySignatureSSZ(publicKey, slot, message, signature); err != nil || !ok {
		t.Errorf("%s: the signature does not verify (%v)", changed, err)
	}
}
