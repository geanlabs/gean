package blockprocessor

import (
	"testing"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func processorAttestationData() *types.AttestationData {
	root := [32]byte{0x01}
	return &types.AttestationData{
		Slot:   1,
		Head:   &types.Checkpoint{Root: root, Slot: 1},
		Target: &types.Checkpoint{Root: root, Slot: 1},
		Source: &types.Checkpoint{Root: root, Slot: 0},
	}
}

func processorAttestation() *types.AggregatedAttestation {
	return &types.AggregatedAttestation{
		AggregationBits: types.NewBitlistSSZ(1),
		Data:            processorAttestationData(),
	}
}

func processorBlock(atts ...*types.AggregatedAttestation) *types.Block {
	return &types.Block{
		Slot:          1,
		ProposerIndex: 0,
		Body:          &types.BlockBody{Attestations: atts},
	}
}

func TestValidateSignedBlockRejectsMalformedEnvelope(t *testing.T) {
	tests := []struct {
		name   string
		block  *types.SignedBlock
		verify bool
	}{
		{name: "nil signed block", block: nil},
		{name: "nil block", block: &types.SignedBlock{}},
		{name: "nil body", block: &types.SignedBlock{Block: &types.Block{}}},
		{name: "missing signatures", block: &types.SignedBlock{Block: processorBlock()}, verify: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block, err := validateSignedBlock(tt.block, tt.verify)
			if err == nil {
				t.Fatalf("expected error, got block=%v", block)
			}
		})
	}
}

func TestValidateBlockAttestationsRejectsMalformedAttestation(t *testing.T) {
	err := validateBlockAttestations(processorBlock(&types.AggregatedAttestation{}))
	if err == nil {
		t.Fatal("expected malformed attestation error")
	}
}

func TestValidateBlockAttestationsRejectsDuplicateData(t *testing.T) {
	data := processorAttestationData()
	block := processorBlock(
		&types.AggregatedAttestation{Data: data},
		&types.AggregatedAttestation{Data: data},
	)

	err := validateBlockAttestations(block)
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrDuplicateAttestationData {
		t.Fatalf("expected duplicate attestation error, got %v", err)
	}
}
