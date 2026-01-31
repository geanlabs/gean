package chain

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/devylongs/gean/types"
)

// GenerateGenesis creates a genesis state with the given parameters.
func GenerateGenesis(genesisTime, numValidators uint64) *types.State {
	emptyBody := &types.BlockBody{Attestations: []types.SignedVote{}}
	bodyRoot, _ := emptyBody.HashTreeRoot()

	genesisHeader := types.BlockHeader{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.Root{},
		StateRoot:     types.Root{},
		BodyRoot:      bodyRoot,
	}

	return &types.State{
		Config: types.Config{
			NumValidators: numValidators,
			GenesisTime:   genesisTime,
		},
		Slot:                    0,
		LatestBlockHeader:       genesisHeader,
		LatestJustified:         types.Checkpoint{Root: types.Root{}, Slot: 0},
		LatestFinalized:         types.Checkpoint{Root: types.Root{}, Slot: 0},
		HistoricalBlockHashes:   []types.Root{},
		JustifiedSlots:          bitfield.NewBitlist(1), // Empty SSZ Bitlist with delimiter
		JustificationRoots:      []types.Root{},
		JustificationValidators: bitfield.NewBitlist(1), // Empty SSZ Bitlist with delimiter
	}
}
