package attestation

import (
	"fmt"

	"github.com/geanlabs/gean/internal/core/crypto/xmss"
	"github.com/geanlabs/gean/internal/lean/types"
)

// Sign signs data with validatorID's attestation key: the XMSS message is the
// attestation data's hash tree root, signed at the data's slot.
func Sign(keys *xmss.KeyManager, validatorID uint64, data *types.AttestationData) ([types.SignatureSize]byte, error) {
	if data == nil {
		return [types.SignatureSize]byte{}, fmt.Errorf("attestation data is nil")
	}
	msgRoot, err := data.HashTreeRoot()
	if err != nil {
		return [types.SignatureSize]byte{}, fmt.Errorf("hash tree root failed: %w", err)
	}
	return keys.SignAttestationRoot(validatorID, data.Slot, msgRoot)
}
