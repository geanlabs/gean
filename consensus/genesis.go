package consensus

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/devylongs/gean/types"
)

// GenerateValidators creates n deterministic placeholder validators.
// Phase 2 uses registry-based validator identities; XMSS key loading is added in later phases.
func GenerateValidators(n int) []types.Validator {
	if n <= 0 {
		return []types.Validator{}
	}

	validators := make([]types.Validator, n)
	for i := 0; i < n; i++ {
		var pk types.Pubkey
		// Deterministic non-zero placeholder pubkey bytes for local/dev tests.
		for j := range pk {
			pk[j] = byte((i + j + 1) % 251)
		}
		validators[i] = types.Validator{
			Pubkey: pk,
			Index:  types.ValidatorIndex(i),
		}
	}
	return validators
}

// GenerateGenesis creates a genesis state and anchor block from the given
// validator set. Bitlists use NewBitlist(0) for empty encoding (sentinel-only).
func GenerateGenesis(genesisTime uint64, validators []types.Validator) (*types.State, *types.Block) {
	emptyBody := types.BlockBody{Attestations: []types.Attestation{}}
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
			GenesisTime: genesisTime,
		},
		Slot:                    0,
		LatestBlockHeader:       genesisHeader,
		LatestJustified:         genesisCheckpoint,
		LatestFinalized:         genesisCheckpoint,
		HistoricalBlockHashes:   []types.Root{},
		JustifiedSlots:          bitfield.NewBitlist(0),
		Validators:              append([]types.Validator{}, validators...),
		JustificationRoots:      []types.Root{},
		JustificationValidators: bitfield.NewBitlist(0),
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
