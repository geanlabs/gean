package genesis

import (
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

func (gc *GenesisConfig) GenesisState() (*types.State, error) {
	validators, err := gc.Validators()
	if err != nil {
		return nil, fmt.Errorf("build genesis validators: %w", err)
	}

	bodyRoot, err := gc.GenesisBody().HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash genesis body: %w", err)
	}

	return &types.State{
		Config: &types.ChainConfig{GenesisTime: gc.GenesisTime},
		Slot:   0,
		LatestBlockHeader: &types.BlockHeader{
			Slot:          0,
			ProposerIndex: 0,
			ParentRoot:    types.ZeroRoot,
			StateRoot:     types.ZeroRoot,
			BodyRoot:      bodyRoot,
		},
		LatestJustified:              &types.Checkpoint{Root: types.ZeroRoot, Slot: 0},
		LatestFinalized:              &types.Checkpoint{Root: types.ZeroRoot, Slot: 0},
		Validators:                   validators,
		JustifiedSlots:               types.NewBitlistSSZ(0),
		JustificationsValidators:     types.NewBitlistSSZ(0),
		LatestExecutionPayloadHeader: gc.genesisExecutionHeader(),
	}, nil
}
