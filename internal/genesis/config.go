// Package genesis provides configuration loading and genesis state generation.
package genesis

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/devylongs/gean/types"
)

// GenesisConfig holds the parameters needed to create a genesis state.
type GenesisConfig struct {
	GenesisTime    uint64 `json:"GENESIS_TIME"`
	ValidatorCount uint64 `json:"VALIDATOR_COUNT"`
}

// configJSON is the intermediate struct for JSON unmarshaling.
// Supports both VALIDATOR_COUNT and deriving count from GENESIS_VALIDATORS.
type configJSON struct {
	GenesisTime       uint64   `json:"GENESIS_TIME"`
	ValidatorCount    uint64   `json:"VALIDATOR_COUNT"`
	GenesisValidators []string `json:"GENESIS_VALIDATORS"`
}

// LoadFromFile loads a GenesisConfig from a JSON file.
func LoadFromFile(path string) (*GenesisConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading genesis file: %w", err)
	}
	return LoadFromJSON(data)
}

// LoadFromJSON loads a GenesisConfig from JSON bytes.
func LoadFromJSON(data []byte) (*GenesisConfig, error) {
	var raw configJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing genesis JSON: %w", err)
	}

	// Use VALIDATOR_COUNT if specified, otherwise derive from GENESIS_VALIDATORS length
	validatorCount := raw.ValidatorCount
	if validatorCount == 0 && len(raw.GenesisValidators) > 0 {
		validatorCount = uint64(len(raw.GenesisValidators))
	}

	config := &GenesisConfig{
		GenesisTime:    raw.GenesisTime,
		ValidatorCount: validatorCount,
	}

	return config, nil
}

// CreateState generates the complete genesis state from this configuration.
func (c *GenesisConfig) CreateState() (*types.State, error) {
	state := GenerateGenesis(c.GenesisTime, c.ValidatorCount)
	return state, nil
}
