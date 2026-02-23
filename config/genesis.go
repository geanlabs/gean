package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/geanlabs/gean/types"
	"gopkg.in/yaml.v3"
)

// GenesisConfig represents the parsed config.yaml for genesis.
type GenesisConfig struct {
	GenesisTime uint64             `yaml:"GENESIS_TIME"`
	Validators  []*types.Validator // populated from GENESIS_VALIDATORS
}

// rawGenesisConfig is the on-disk YAML shape.
type rawGenesisConfig struct {
	GenesisTime       uint64   `yaml:"GENESIS_TIME"`
	GenesisValidators []string `yaml:"GENESIS_VALIDATORS"`
}

// LoadGenesisConfig loads and parses a genesis config YAML file.
func LoadGenesisConfig(path string) (*GenesisConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var raw rawGenesisConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if len(raw.GenesisValidators) == 0 {
		return nil, fmt.Errorf("GENESIS_VALIDATORS must not be empty")
	}

	validators := make([]*types.Validator, len(raw.GenesisValidators))
	for i, hexStr := range raw.GenesisValidators {
		hexStr = strings.TrimPrefix(hexStr, "0x")
		pubkeyBytes, err := hex.DecodeString(hexStr)
		if err != nil {
			return nil, fmt.Errorf("invalid pubkey hex at index %d: %w", i, err)
		}
		if len(pubkeyBytes) != 52 {
			return nil, fmt.Errorf("pubkey at index %d is %d bytes, want 52", i, len(pubkeyBytes))
		}
		var pubkey [52]byte
		copy(pubkey[:], pubkeyBytes)
		validators[i] = &types.Validator{Pubkey: pubkey, Index: uint64(i)}
	}

	return &GenesisConfig{
		GenesisTime: raw.GenesisTime,
		Validators:  validators,
	}, nil
}
