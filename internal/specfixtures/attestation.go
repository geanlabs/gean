package specfixtures

import (
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

func (ta *TestAggregatedAttestation) ToAggregatedAttestation() (*types.AggregatedAttestation, error) {
	if ta == nil {
		return nil, fmt.Errorf("aggregated attestation fixture is nil")
	}
	bits, err := ParseBoolBitlist(ta.AggregationBits.Data)
	if err != nil {
		return nil, fmt.Errorf("aggregationBits: %w", err)
	}
	data, err := ta.Data.ToAttestationData()
	if err != nil {
		return nil, err
	}
	return &types.AggregatedAttestation{AggregationBits: bits, Data: data}, nil
}

func (tad *TestAttData) ToAttestationData() (*types.AttestationData, error) {
	if tad == nil {
		return nil, fmt.Errorf("attestation data fixture is nil")
	}
	headRoot, err := ParseHexRoot(tad.Head.Root)
	if err != nil {
		return nil, fmt.Errorf("data.head.root: %w", err)
	}
	targetRoot, err := ParseHexRoot(tad.Target.Root)
	if err != nil {
		return nil, fmt.Errorf("data.target.root: %w", err)
	}
	sourceRoot, err := ParseHexRoot(tad.Source.Root)
	if err != nil {
		return nil, fmt.Errorf("data.source.root: %w", err)
	}
	return &types.AttestationData{
		Slot:   tad.Slot,
		Head:   &types.Checkpoint{Root: headRoot, Slot: tad.Head.Slot},
		Target: &types.Checkpoint{Root: targetRoot, Slot: tad.Target.Slot},
		Source: &types.Checkpoint{Root: sourceRoot, Slot: tad.Source.Slot},
	}, nil
}
