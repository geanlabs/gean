package leansig

import (
	"crypto/rand"
	"testing"
)

// Devnet-1 parameters for SIGTopLevelTargetSumLifetime32Dim64Base8:
// LOG_LIFETIME=32, sqrt(LIFETIME)=65536, min active range = 2*65536 = 131072
// Devnet-1 spec uses activation_time = 2^3 = 8
const testActivationEpoch = 0
const testNumActiveEpochs = 262144 // 2^3, matching devnet-1 spec

// TestKeyGeneration verifies that keypair generation succeeds and returns
// valid activation and prepared intervals.
func TestKeyGeneration(t *testing.T) {
	kp, err := GenerateKeypair(42, testActivationEpoch, testNumActiveEpochs)
	if err != nil {
		t.Fatalf("GenerateKeypair failed: %v", err)
	}
	defer kp.Free()

	t.Logf("Activation interval: [%d, %d)", kp.ActivationStart(), kp.ActivationEnd())
	t.Logf("Prepared interval: [%d, %d)", kp.PreparedStart(), kp.PreparedEnd())

	if kp.ActivationEnd() <= kp.ActivationStart() {
		t.Errorf("activation interval is empty or invalid")
	}
	if kp.PreparedEnd() <= kp.PreparedStart() {
		t.Errorf("prepared interval is empty or invalid")
	}
}

// TestKeySerializationRoundtrip tests that public key serialization produces
// non-empty bytes.
func TestKeySerializationRoundtrip(t *testing.T) {
	kp, err := GenerateKeypair(42, testActivationEpoch, testNumActiveEpochs)
	if err != nil {
		t.Fatalf("GenerateKeypair failed: %v", err)
	}
	defer kp.Free()

	pkBytes, err := kp.PublicKeyBytes()
	if err != nil {
		t.Fatalf("PublicKeyBytes failed: %v", err)
	}
	if len(pkBytes) == 0 {
		t.Fatal("public key bytes are empty")
	}
	t.Logf("Public key size: %d bytes", len(pkBytes))

	skBytes, err := kp.SecretKeyBytes()
	if err != nil {
		t.Fatalf("SecretKeyBytes failed: %v", err)
	}
	if len(skBytes) == 0 {
		t.Fatal("secret key bytes are empty")
	}
	t.Logf("Secret key size: %d bytes", len(skBytes))
}

// TestSignAndVerifyWithKeypair tests the full sign and verify roundtrip using
// the keypair handle for verification.
func TestSignAndVerifyWithKeypair(t *testing.T) {
	kp, err := GenerateKeypair(42, testActivationEpoch, testNumActiveEpochs)
	if err != nil {
		t.Fatalf("GenerateKeypair failed: %v", err)
	}
	defer kp.Free()

	epoch := uint32(0)
	var msg [MessageLength]byte
	if _, err := rand.Read(msg[:]); err != nil {
		t.Fatalf("rand.Read failed: %v", err)
	}

	sig, err := kp.Sign(epoch, msg)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	t.Logf("Signature size: %d bytes", len(sig))

	err = kp.VerifyWithKeypair(epoch, msg, sig)
	if err != nil {
		t.Fatalf("VerifyWithKeypair failed: %v", err)
	}
	t.Log("Signature verified with keypair ✓")
}

// TestSignAndVerifyWithSerializedPubkey tests the full sign and verify roundtrip
// using serialized public key bytes (the path used in consensus).
func TestSignAndVerifyWithSerializedPubkey(t *testing.T) {
	kp, err := GenerateKeypair(42, testActivationEpoch, testNumActiveEpochs)
	if err != nil {
		t.Fatalf("GenerateKeypair failed: %v", err)
	}
	defer kp.Free()

	pkBytes, err := kp.PublicKeyBytes()
	if err != nil {
		t.Fatalf("PublicKeyBytes failed: %v", err)
	}

	epoch := uint32(0)
	var msg [MessageLength]byte
	copy(msg[:], []byte("test message for devnet-1 xmss"))

	sig, err := kp.Sign(epoch, msg)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	err = Verify(pkBytes, epoch, msg, sig)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	t.Log("Signature verified with serialized pubkey ✓")
}

// TestVerifyRejectsWrongMessage tests that verification fails for a wrong message.
func TestVerifyRejectsWrongMessage(t *testing.T) {
	kp, err := GenerateKeypair(42, testActivationEpoch, testNumActiveEpochs)
	if err != nil {
		t.Fatalf("GenerateKeypair failed: %v", err)
	}
	defer kp.Free()

	epoch := uint32(0)
	var msg [MessageLength]byte
	copy(msg[:], []byte("correct message"))

	sig, err := kp.Sign(epoch, msg)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Tamper with the message
	var wrongMsg [MessageLength]byte
	copy(wrongMsg[:], []byte("wrong message!!"))

	err = kp.VerifyWithKeypair(epoch, wrongMsg, sig)
	if err == nil {
		t.Fatal("Expected verification to fail with wrong message, but it succeeded")
	}
	t.Logf("Correctly rejected wrong message: %v ✓", err)
}

// TestVerifyRejectsWrongEpoch tests that verification fails at a wrong epoch.
func TestVerifyRejectsWrongEpoch(t *testing.T) {
	kp, err := GenerateKeypair(42, testActivationEpoch, testNumActiveEpochs)
	if err != nil {
		t.Fatalf("GenerateKeypair failed: %v", err)
	}
	defer kp.Free()

	epoch := uint32(0)
	var msg [MessageLength]byte
	copy(msg[:], []byte("epoch test message"))

	sig, err := kp.Sign(epoch, msg)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Verify with wrong epoch
	err = kp.VerifyWithKeypair(epoch+1, msg, sig)
	if err == nil {
		t.Fatal("Expected verification to fail with wrong epoch, but it succeeded")
	}
	t.Logf("Correctly rejected wrong epoch: %v ✓", err)
}

// TestAdvancePreparation tests that the preparation window can be advanced.
// With LOG_LIFETIME=32, sqrt(LIFETIME)=65536, prepared_window=2*65536=131072.
// We need num_active_epochs > 131072 for advance to work (262144 gives 4 bottom trees).
func TestAdvancePreparation(t *testing.T) {
	kp, err := GenerateKeypair(42, testActivationEpoch, testNumActiveEpochs)
	if err != nil {
		t.Fatalf("GenerateKeypair failed: %v", err)
	}
	defer kp.Free()

	startBefore := kp.PreparedStart()
	endBefore := kp.PreparedEnd()
	t.Logf("Before advance: [%d, %d)", startBefore, endBefore)

	err = kp.AdvancePreparation()
	if err != nil {
		t.Fatalf("AdvancePreparation failed: %v", err)
	}

	startAfter := kp.PreparedStart()
	endAfter := kp.PreparedEnd()
	t.Logf("After advance:  [%d, %d)", startAfter, endAfter)

	if startAfter <= startBefore {
		t.Errorf("prepared start did not advance: before=%d after=%d", startBefore, startAfter)
	}
	if endAfter <= endBefore {
		t.Errorf("prepared end did not advance: before=%d after=%d", endBefore, endAfter)
	}
}
