package genesis

import "github.com/devylongs/gean/types"

// GenerateGenesis creates the genesis state at slot 0.
//
// The genesis state has:
//   - slot = 0
//   - Empty historical data (block hashes, justified slots, justification tracking)
//   - Genesis block header with zeroed state/parent roots
//   - Default checkpoints at slot 0 with zero root
func GenerateGenesis(genesisTime uint64, numValidators uint64) *types.State {
	// Create config with genesis time and number of validators
	config := types.Config{
		NumValidators: numValidators,
		GenesisTime:   genesisTime,
	}

	// Calculate the body root for an empty block body
	emptyBody := types.BlockBody{
		Attestations: []types.SignedVote{},
	}
	bodyRoot, err := emptyBody.HashTreeRoot()
	if err != nil {
		// This should never fail for an empty body
		panic("failed to compute empty body root: " + err.Error())
	}

	// Build the genesis block header
	genesisHeader := types.BlockHeader{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.Root{},
		StateRoot:     types.Root{},
		BodyRoot:      bodyRoot,
	}

	// Default checkpoint at slot 0 with zero root
	defaultCheckpoint := types.Checkpoint{
		Slot: 0,
		Root: types.Root{},
	}

	// Bitlists need a sentinel byte (0x01) to be valid non-empty
	justifiedSlots := []byte{0x01}
	justificationsValidators := []byte{0x01}

	// Assemble and return the full genesis state
	return &types.State{
		Config:                   config,
		Slot:                     0,
		LatestBlockHeader:        genesisHeader,
		LatestJustified:          defaultCheckpoint,
		LatestFinalized:          defaultCheckpoint,
		HistoricalBlockHashes:    []types.Root{},
		JustifiedSlots:           justifiedSlots,
		JustificationsRoots:      []types.Root{},
		JustificationsValidators: justificationsValidators,
	}
}
