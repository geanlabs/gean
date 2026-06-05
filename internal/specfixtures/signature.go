package specfixtures

import (
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

func (sb *FixtureSignedBlock) ToSignedBlock() (*types.SignedBlock, error) {
	if sb == nil {
		return nil, fmt.Errorf("signed block fixture is nil")
	}
	srcBlock := sb.Block
	if (srcBlock.Slot == 0 && srcBlock.ParentRoot == "") && sb.Message != nil {
		srcBlock = sb.Message.Block
	}
	block, err := srcBlock.ToBlock()
	if err != nil {
		return nil, fmt.Errorf("signedBlock.block: %w", err)
	}

	proposerSig, err := ParseHexSignature(sb.Signature.ProposerSignature)
	if err != nil {
		return nil, fmt.Errorf("signedBlock.signature.proposerSignature: %w", err)
	}

	var attSigs []*types.AggregatedSignatureProof
	for i, proof := range sb.Signature.AttestationSignatures.Data {
		participants, perr := ParseBoolBitlist(proof.Participants.Data)
		if perr != nil {
			return nil, fmt.Errorf("signedBlock.signature.attestationSignatures[%d].participants: %w", i, perr)
		}
		proofData, perr := ParseHexBytes(proof.ProofData.Data)
		if perr != nil {
			return nil, fmt.Errorf("signedBlock.signature.attestationSignatures[%d].proofData: %w", i, perr)
		}
		attSigs = append(attSigs, &types.AggregatedSignatureProof{Participants: participants, ProofData: proofData})
	}

	return &types.SignedBlock{
		Block: block,
		Signature: &types.BlockSignatures{
			ProposerSignature:     proposerSig,
			AttestationSignatures: attSigs,
		},
	}, nil
}
