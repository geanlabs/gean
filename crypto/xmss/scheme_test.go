package xmss

import (
	"fmt"
	"testing"

	"github.com/geanlabs/gean/crypto"
	"github.com/geanlabs/gean/crypto/schemetest"
)

func TestSchemeConformance(t *testing.T) {
	scheme := NewScheme()
	defer scheme.Close()
	schemetest.Run(t, scheme, func(t *testing.T, i int) (crypto.PublicKey, func(uint32, [32]byte) crypto.Signature) {
		kp, err := GenerateKeyPair(fmt.Sprintf("scheme-conformance-%d", i), 0, 1<<10)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		t.Cleanup(kp.Close)
		pk, err := kp.PublicKeyBytes()
		if err != nil {
			t.Fatalf("public key: %v", err)
		}
		return pk, func(slot uint32, message [32]byte) crypto.Signature {
			sig, err := kp.Sign(slot, message)
			if err != nil {
				t.Fatalf("sign: %v", err)
			}
			return sig
		}
	})
}
