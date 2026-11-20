//go:build !fakecrypto

package xmss

import (
	"strings"
	"testing"
)

func TestValidatorKeyPairRejectsClosedKey(t *testing.T) {
	kp, err := GenerateKeyPair("test-closed-keypair", 0, 1<<10)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	kp.Close()

	if kp.PublicKeyPtr() != nil {
		t.Fatal("closed keypair should not expose public key pointer")
	}
	if kp.PrivateKeyPtr() != nil {
		t.Fatal("closed keypair should not expose private key pointer")
	}
	if _, err := kp.Sign(0, [32]byte{}); err == nil || !strings.Contains(err.Error(), "keypair is nil or closed") {
		t.Fatalf("Sign error=%v, want closed keypair error", err)
	}
	if _, err := kp.PublicKeyBytes(); err == nil || !strings.Contains(err.Error(), "keypair is nil or closed") {
		t.Fatalf("PublicKeyBytes error=%v, want closed keypair error", err)
	}
	if _, err := kp.PrivateKeyBytes(); err == nil || !strings.Contains(err.Error(), "keypair is nil or closed") {
		t.Fatalf("PrivateKeyBytes error=%v, want closed keypair error", err)
	}
}
