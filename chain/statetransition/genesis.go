package statetransition

import (
	"github.com/devylongs/gean/types"
)

// GenerateGenesis creates a genesis state with the given parameters.
func GenerateGenesis(genesisTime, numValidators uint64) *types.State {
	config := &types.Config{
		NumValidators: numValidators,
		GenesisTime:   genesisTime,
	}

	emptyBody := &types.BlockBody{Attestations: []*types.SignedVote{}}
	bodyRoot, _ := emptyBody.HashTreeRoot()

	genesisHeader := &types.BlockHeader{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash,
		BodyRoot:      bodyRoot,
	}

	return &types.State{
		Config:                   config,
		Slot:                     0,
		LatestBlockHeader:        genesisHeader,
		LatestJustified:          &types.Checkpoint{Root: types.ZeroHash, Slot: 0},
		LatestFinalized:          &types.Checkpoint{Root: types.ZeroHash, Slot: 0},
		HistoricalBlockHashes:    [][32]byte{},
		JustifiedSlots:           []byte{0x01}, // empty bitlist with sentinel
		JustificationsRoots:      [][32]byte{},
		JustificationsValidators: []byte{0x01}, // empty bitlist with sentinel
	}
}
