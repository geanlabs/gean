package test

import (
	"crypto/rand"
	"path/filepath"
	"testing"

	"github.com/geanlabs/gean/xmss/leansig"
)

const (
	testActivationEpoch = 0
	testNumActiveEpochs = 8
)

func TestSaveAndLoadKeypair(t *testing.T) {
	t.Logf("Generating keypair (this may take ~30s)...")
	kp, err := leansig.GenerateKeypair(999, 0, testNumActiveEpochs)
	if err != nil {
		t.Fatalf("GenerateKeypair failed: %v", err)
	}
	defer kp.Free()

	// 2. Sign some message
	var msg [leansig.MessageLength]byte
	if _, err := rand.Read(msg[:]); err != nil {
		t.Fatalf("rand.Read failed: %v", err)
	}

	t.Log("Signing message with original keypair...")
	epoch := uint32(0)
	sigOriginal, err := kp.Sign(epoch, msg)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// 3. Save to temp dir
	dir := t.TempDir()
	pkPath := filepath.Join(dir, "validator_test.pk")
	skPath := filepath.Join(dir, "validator_test.sk")

	t.Logf("Saving keypair to %s", dir)
	if err := leansig.SaveKeypair(kp, pkPath, skPath); err != nil {
		t.Fatalf("SaveKeypair failed: %v", err)
	}

	// 4. Load back
	t.Log("Loading keypair back from disk...")
	kpLoaded, err := leansig.LoadKeypair(pkPath, skPath)
	if err != nil {
		t.Fatalf("LoadKeypair failed: %v", err)
	}
	defer kpLoaded.Free()

	// 5. Verify original signature with loaded keypair
	t.Log("Verifying original signature with loaded keypair...")
	if err := kpLoaded.VerifyWithKeypair(epoch, msg, sigOriginal); err != nil {
		t.Errorf("Verify with loaded keypair failed: %v", err)
	}

	// 6. Sign with loaded keypair
	t.Log("Signing new message with loaded keypair...")
	sigNew, err := kpLoaded.Sign(epoch, msg)
	if err != nil {
		t.Fatalf("Sign with loaded keypair failed: %v", err)
	}

	// 7. Verify new signature with original keypair
	t.Log("Verifying new signature with original keypair...")
	if err := kp.VerifyWithKeypair(epoch, msg, sigNew); err != nil {
		t.Errorf("Verify new signature with original keypair failed: %v", err)
	}

	t.Log("Key persistence test passed ✓")
}
