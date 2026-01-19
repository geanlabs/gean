// Package genesis provides configuration loading and genesis state generation.
package genesis

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/devylongs/gean/types"
)

// GenesisConfig holds the parameters needed to create a genesis state.
type GenesisConfig struct {
	GenesisTime       uint64          `json:"GENESIS_TIME"`
	GenesisValidators []types.Bytes52 `json:"GENESIS_VALIDATORS"`
}

// configJSON is the intermediate struct for JSON unmarshaling.
type configJSON struct {
	GenesisTime       uint64   `json:"GENESIS_TIME"`
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

	config := &GenesisConfig{
		GenesisTime:       raw.GenesisTime,
		GenesisValidators: make([]types.Bytes52, len(raw.GenesisValidators)),
	}

	for i, hexStr := range raw.GenesisValidators {
		pubkey, err := parseHexPubkey(hexStr)
		if err != nil {
			return nil, fmt.Errorf("parsing validator %d pubkey: %w", i, err)
		}
		config.GenesisValidators[i] = pubkey
	}

	return config, nil
}

// parseHexPubkey converts a hex string (with or without 0x prefix) to Bytes52.
func parseHexPubkey(s string) (types.Bytes52, error) {
	s = strings.TrimPrefix(s, "0x")
	if len(s) != 104 { // 52 bytes = 104 hex chars
		return types.Bytes52{}, fmt.Errorf("invalid pubkey length: got %d hex chars, want 104", len(s))
	}

	decoded, err := hex.DecodeString(s)
	if err != nil {
		return types.Bytes52{}, fmt.Errorf("decoding hex: %w", err)
	}

	var pubkey types.Bytes52
	copy(pubkey[:], decoded)
	return pubkey, nil
}

// ToValidators converts genesis pubkeys to Validator structs with indices.
func (c *GenesisConfig) ToValidators() []types.Validator {
	validators := make([]types.Validator, len(c.GenesisValidators))
	for i, pk := range c.GenesisValidators {
		validators[i] = types.Validator{
			Pubkey: pk,
			Index:  uint64(i),
		}
	}
	return validators
}

// CreateState generates the complete genesis state from this configuration.
func (c *GenesisConfig) CreateState() (*types.State, error) {
	validators := c.ToValidators()
	state := GenerateGenesis(c.GenesisTime, validators)
	return state, nil
}
