package genesis

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/geanlabs/gean/internal/types"
)

func (gc *GenesisConfig) Validators() ([]*types.Validator, error) {
	if err := gc.validate(); err != nil {
		return nil, err
	}

	validators := make([]*types.Validator, len(gc.GenesisValidators))
	for i, entry := range gc.GenesisValidators {
		attestationPubkey, err := decodePubkey(entry.AttestationPubkey, i, "attestation")
		if err != nil {
			return nil, err
		}
		proposalPubkey, err := decodePubkey(entry.ProposalPubkey, i, "proposal")
		if err != nil {
			return nil, err
		}
		validators[i] = &types.Validator{
			AttestationPubkey: attestationPubkey,
			ProposalPubkey:    proposalPubkey,
			Index:             uint64(i),
		}
	}
	return validators, nil
}

func decodePubkey(hexStr string, index int, keyType string) ([types.PubkeySize]byte, error) {
	normalized := strings.TrimSpace(hexStr)
	normalized = strings.TrimPrefix(strings.TrimPrefix(normalized, "0x"), "0X")
	pkBytes, err := hex.DecodeString(normalized)
	if err != nil || len(pkBytes) != types.PubkeySize {
		return [types.PubkeySize]byte{}, fmt.Errorf("GENESIS_VALIDATORS[%d] invalid %s pubkey: %s", index, keyType, normalized)
	}

	var pubkey [types.PubkeySize]byte
	copy(pubkey[:], pkBytes)
	if pubkey == ([types.PubkeySize]byte{}) {
		return [types.PubkeySize]byte{}, fmt.Errorf("GENESIS_VALIDATORS[%d] invalid %s pubkey: zero pubkey", index, keyType)
	}
	return pubkey, nil
}
