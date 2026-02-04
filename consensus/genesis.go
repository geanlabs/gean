package consensus

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/devylongs/gean/types"
)

// GenerateGenesis creates a genesis state and block with the given parameters.
func GenerateGenesis(genesisTime, numValidators uint64) (*types.State, *types.Block) {
	emptyBody := types.BlockBody{Attestations: []types.SignedVote{}}
	bodyRoot, _ := emptyBody.HashTreeRoot()

	genesisHeader := types.BlockHeader{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.Root{},
		StateRoot:     types.Root{},
		BodyRoot:      bodyRoot,
	}

	// Genesis checkpoints use zero root - the store handles this as a special case
	genesisCheckpoint := types.Checkpoint{Root: types.Root{}, Slot: 0}

	state := &types.State{
		Config: types.Config{
			NumValidators: numValidators,
			GenesisTime:   genesisTime,
		},
		Slot:                    0,
		LatestBlockHeader:       genesisHeader,
		LatestJustified:         genesisCheckpoint,
		LatestFinalized:         genesisCheckpoint,
		HistoricalBlockHashes:   []types.Root{},
		JustifiedSlots:          bitfield.NewBitlist(1),
		JustificationRoots:      []types.Root{},
		JustificationValidators: bitfield.NewBitlist(1),
	}

	stateRoot, _ := state.HashTreeRoot()

	block := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.Root{},
		StateRoot:     stateRoot,
		Body:          emptyBody,
	}

	return state, block
}
