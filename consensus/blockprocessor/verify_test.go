package blockprocessor

import (
	"testing"

	"github.com/geanlabs/gean/crypto/insecure"
	"github.com/geanlabs/gean/storage/store"
	"github.com/geanlabs/gean/types"
)

func TestVerifyBlockSignaturesRejectsMalformedBlocks(t *testing.T) {

	overflowAtt := processorAttestation()
	overflowAtt.Data.Slot = ^uint64(0)
	types.BitlistSet(overflowAtt.AggregationBits, 0)
	overflowBlock := processorBlock()
	overflowBlock.Slot = ^uint64(0)
	proof := &types.MultiMessageAggregate{Proof: []byte{1}}
	oneValidator := &types.State{Validators: []*types.Validator{{}}}

	tests := []struct {
		name  string
		block *types.SignedBlock
		state *types.State
		want  store.StoreErrorKind
	}{
		{"missing proof", &types.SignedBlock{Block: processorBlock(), Proof: &types.MultiMessageAggregate{}}, oneValidator, store.ErrAttestationSignatureMismatch},
		{"empty participants", &types.SignedBlock{Block: processorBlock(processorAttestation()), Proof: proof}, oneValidator, store.ErrParticipantsMismatch},
		{"attestation slot overflow", &types.SignedBlock{Block: processorBlock(overflowAtt), Proof: proof}, oneValidator, store.ErrSignatureDecodingFailed},
		{"block slot overflow", &types.SignedBlock{Block: overflowBlock, Proof: proof}, oneValidator, store.ErrProposerSignatureDecodingFailed},
		{"nil proposer validator", &types.SignedBlock{Block: processorBlock(), Proof: proof}, &types.State{Validators: []*types.Validator{nil}}, store.ErrInvalidValidatorIndex},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyBlockSignatures(insecure.Scheme{}, tt.block, tt.state)
			se, ok := err.(*store.StoreError)
			if !ok || se.Kind != tt.want {
				t.Fatalf("error=%v, want kind %v", err, tt.want)
			}
		})
	}
}
