package specfixtures

import (
	"encoding/json"
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

func (tb *TestBlock) ToBlock() (*types.Block, error) {
	if tb == nil {
		return nil, fmt.Errorf("block fixture is nil")
	}
	parentRoot, err := ParseHexRoot(tb.ParentRoot)
	if err != nil {
		return nil, fmt.Errorf("block.parentRoot: %w", err)
	}
	stateRoot, err := ParseHexRoot(tb.StateRoot)
	if err != nil {
		return nil, fmt.Errorf("block.stateRoot: %w", err)
	}

	block := &types.Block{
		Slot:          tb.Slot,
		ProposerIndex: tb.ProposerIndex,
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body:          &types.BlockBody{Attestations: make([]*types.AggregatedAttestation, 0)},
	}

	for i, raw := range tb.Body.Attestations.Data {
		var ta TestAggregatedAttestation
		if err := json.Unmarshal(raw, &ta); err != nil {
			return nil, fmt.Errorf("block.body.attestations[%d]: %w", i, err)
		}
		att, err := ta.ToAggregatedAttestation()
		if err != nil {
			return nil, fmt.Errorf("block.body.attestations[%d]: %w", i, err)
		}
		block.Body.Attestations = append(block.Body.Attestations, att)
	}

	return block, nil
}
