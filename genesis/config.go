package genesis

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/geanlabs/gean/types"
	"gopkg.in/yaml.v3"
)

// GenesisConfig is parsed from config.yaml.
// Parsed from config.yaml.
type GenesisConfig struct {
	GenesisTime       uint64   `yaml:"GENESIS_TIME"`
	GenesisValidators []string `yaml:"GENESIS_VALIDATORS"`
}

// Validators converts hex pubkey strings to typed Validators with sequential indices.
func (gc *GenesisConfig) Validators() []*types.Validator {
	validators := make([]*types.Validator, len(gc.GenesisValidators))
	for i, hexStr := range gc.GenesisValidators {
		hexStr = strings.TrimPrefix(strings.TrimSpace(hexStr), "0x")
		pkBytes, err := hex.DecodeString(hexStr)
		if err != nil || len(pkBytes) != types.PubkeySize {
			panic(fmt.Sprintf("GENESIS_VALIDATORS[%d] invalid: %s", i, hexStr))
		}
		var pubkey [types.PubkeySize]byte
		copy(pubkey[:], pkBytes)
		validators[i] = &types.Validator{
			Pubkey: pubkey,
			Index:  uint64(i),
		}
	}
	return validators
}

// GenesisState creates the genesis state from config.
func (gc *GenesisConfig) GenesisState() *types.State {
	validators := gc.Validators()

	// Genesis block header with empty body root.
	emptyBody := &types.BlockBody{}
	bodyRoot, _ := emptyBody.HashTreeRoot()

	return &types.State{
		Config:            &types.ChainConfig{GenesisTime: gc.GenesisTime},
		Slot:              0,
		LatestBlockHeader: &types.BlockHeader{
			Slot:          0,
			ProposerIndex: 0,
			ParentRoot:    types.ZeroRoot,
			StateRoot:     types.ZeroRoot,
			BodyRoot:      bodyRoot,
		},
		LatestJustified:          &types.Checkpoint{Root: types.ZeroRoot, Slot: 0},
		LatestFinalized:          &types.Checkpoint{Root: types.ZeroRoot, Slot: 0},
		Validators:               validators,
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
}

// LoadGenesisConfig reads and parses config.yaml.
func LoadGenesisConfig(path string) (*GenesisConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config.yaml: %w", err)
	}
	var config GenesisConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse config.yaml: %w", err)
	}
	if config.GenesisTime == 0 {
		return nil, fmt.Errorf("GENESIS_TIME is 0 or missing")
	}
	if len(config.GenesisValidators) == 0 {
		return nil, fmt.Errorf("GENESIS_VALIDATORS is empty")
	}
	return &config, nil
}
