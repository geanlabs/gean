package leansig

import (
	"fmt"
	"os"
)

// LoadKeypair reads public and secret keys from disk and restores the Keypair handle.
// Both files must exist and contain valid SSZ-serialized key data.
func LoadKeypair(pkPath, skPath string) (*Keypair, error) {
	pkBytes, err := os.ReadFile(pkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key from %s: %w", pkPath, err)
	}

	skBytes, err := os.ReadFile(skPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret key from %s: %w", skPath, err)
	}

	return RestoreKeypair(pkBytes, skBytes)
}

// SaveKeypair writes the public and secret keys of a Keypair to disk.
// Public key is written with 0644 permissions, secret key with 0600.
func SaveKeypair(kp *Keypair, pkPath, skPath string) error {
	pkBytes, err := kp.PublicKeyBytes()
	if err != nil {
		return fmt.Errorf("failed to serialize public key: %w", err)
	}

	skBytes, err := kp.SecretKeyBytes()
	if err != nil {
		return fmt.Errorf("failed to serialize secret key: %w", err)
	}

	if err := os.WriteFile(pkPath, pkBytes, 0644); err != nil {
		return fmt.Errorf("failed to write public key to %s: %w", pkPath, err)
	}

	// Secret key gets restrictive permissions
	if err := os.WriteFile(skPath, skBytes, 0600); err != nil {
		return fmt.Errorf("failed to write secret key to %s: %w", skPath, err)
	}

	return nil
}
