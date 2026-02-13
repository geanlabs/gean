package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// GenesisConfig represents the parsed config.yaml for genesis.
type GenesisConfig struct {
	GenesisTime    uint64 `yaml:"GENESIS_TIME"`
	ValidatorCount uint64 `yaml:"VALIDATOR_COUNT"`
}

// LoadGenesisConfig loads and parses a genesis config YAML file.
func LoadGenesisConfig(path string) (*GenesisConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg GenesisConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.ValidatorCount == 0 {
		return nil, fmt.Errorf("VALIDATOR_COUNT must be > 0")
	}

	return &cfg, nil
}
