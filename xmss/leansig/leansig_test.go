package leansig_test

import (
	"crypto/rand"
	"fmt"
	"os"
	"testing"

	"github.com/geanlabs/gean/xmss/leansig"
)

// Devnet-1 parameters for SIGTopLevelTargetSumLifetime32Dim64Base8:
// LOG_LIFETIME=32, sqrt(LIFETIME)=65536, min active range = 2*65536 = 131072
// Devnet-1 spec uses activation_time = 2^3 = 8
const testLsigActivationEpoch = 0
const testLsigNumActiveEpochs = 262144 // 2^18, matching devnet-1 spec

// Shared keypair generated once in TestMain to avoid redundant ~80s keygen per test.
var sharedKP *leansig.Keypair

func TestMain(m *testing.M) {
	var err error
	sharedKP, err = leansig.GenerateKeypair(42, testLsigActivationEpoch, testLsigNumActiveEpochs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: GenerateKeypair failed: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	sharedKP.Free()
	os.Exit(code)
}

// TestKeyGeneration verifies that keypair generation succeeds and returns
// valid activation and prepared intervals.
func TestKeyGeneration(t *testing.T) {
	if sharedKP.ActivationEnd() <= sharedKP.ActivationStart() {
		t.Errorf("activation interval is empty or invalid")
	}
	if sharedKP.PreparedEnd() <= sharedKP.PreparedStart() {
		t.Errorf("prepared interval is empty or invalid")
	}
	t.Logf("Activation interval: [%d, %d)", sharedKP.ActivationStart(), sharedKP.ActivationEnd())
	t.Logf("Prepared interval: [%d, %d)", sharedKP.PreparedStart(), sharedKP.PreparedEnd())
}

func TestKeySerializationRoundtrip(t *testing.T) {
	pkBytes, err := sharedKP.PublicKeyBytes()
	if err != nil {
		t.Fatalf("PublicKeyBytes failed: %v", err)
	}
	if len(pkBytes) == 0 {
		t.Fatal("public key bytes are empty")
	}
	t.Logf("Public key size: %d bytes", len(pkBytes))

	skBytes, err := sharedKP.SecretKeyBytes()
	if err != nil {
		t.Fatalf("SecretKeyBytes failed: %v", err)
	}
	if len(skBytes) == 0 {
		t.Fatal("secret key bytes are empty")
	}
	t.Logf("Secret key size: %d bytes", len(skBytes))
}

func TestSignAndVerifyWithKeypair(t *testing.T) {
	epoch := uint32(0)
	var msg [leansig.MessageLength]byte
	if _, err := rand.Read(msg[:]); err != nil {
		t.Fatalf("rand.Read failed: %v", err)
	}

	sig, err := sharedKP.Sign(epoch, msg)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	t.Logf("Signature size: %d bytes", len(sig))

	err = sharedKP.VerifyWithKeypair(epoch, msg, sig)
	if err != nil {
		t.Fatalf("VerifyWithKeypair failed: %v", err)
	}
}

func TestSignAndVerifyWithSerializedPubkey(t *testing.T) {
	pkBytes, err := sharedKP.PublicKeyBytes()
	if err != nil {
		t.Fatalf("PublicKeyBytes failed: %v", err)
	}

	epoch := uint32(0)
	var msg [leansig.MessageLength]byte
	copy(msg[:], []byte("test message for devnet-1 xmss"))

	sig, err := sharedKP.Sign(epoch, msg)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	err = leansig.Verify(pkBytes, epoch, msg, sig)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
}

func TestVerifyRejectsWrongMessage(t *testing.T) {
	epoch := uint32(0)
	var msg [leansig.MessageLength]byte
	copy(msg[:], []byte("correct message"))

	sig, err := sharedKP.Sign(epoch, msg)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	var wrongMsg [leansig.MessageLength]byte
	copy(wrongMsg[:], []byte("wrong message!!"))

	err = sharedKP.VerifyWithKeypair(epoch, wrongMsg, sig)
	if err == nil {
		t.Fatal("Expected verification to fail with wrong message, but it succeeded")
	}
}

func TestVerifyRejectsWrongEpoch(t *testing.T) {
	epoch := uint32(0)
	var msg [leansig.MessageLength]byte
	copy(msg[:], []byte("epoch test message"))

	sig, err := sharedKP.Sign(epoch, msg)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	err = sharedKP.VerifyWithKeypair(epoch+1, msg, sig)
	if err == nil {
		t.Fatal("Expected verification to fail with wrong epoch, but it succeeded")
	}
}

func TestAdvancePreparation(t *testing.T) {
	// We need > 131072 epochs to trigger window advancement.
	// 200000 epochs roughly covers 1.5 windows.
	const largeNumEpochs = 200000
	t.Logf("Generating large keypair for advance test (%d epochs)...", largeNumEpochs)
	kp, err := leansig.GenerateKeypair(42, testLsigActivationEpoch, largeNumEpochs)
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
