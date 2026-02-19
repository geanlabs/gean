package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ValidatorAssignment maps a node name to its validator indices.
type ValidatorAssignment struct {
	NodeName   string   `yaml:"node_name"`
	Validators []uint64 `yaml:"validators"`
}

// ValidatorRegistry is the parsed validators.yaml.
type ValidatorRegistry struct {
	Assignments []ValidatorAssignment `yaml:"assignments"`
}

// LoadValidators loads and parses a validators.yaml file.
func LoadValidators(path string) (*ValidatorRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read validators: %w", err)
	}

	var reg ValidatorRegistry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parse validators: %w", err)
	}

	return &reg, nil
}

// Validate checks for overlapping assignments and out-of-range indices.
func (r *ValidatorRegistry) Validate(numGenesisValidators uint64) error {
	seen := make(map[uint64]string)
	for _, a := range r.Assignments {
		for _, idx := range a.Validators {
			if idx >= numGenesisValidators {
				return fmt.Errorf("validator %d in %s out of range (genesis has %d)", idx, a.NodeName, numGenesisValidators)
			}
			if prev, ok := seen[idx]; ok {
				return fmt.Errorf("validator %d assigned to both %s and %s", idx, prev, a.NodeName)
			}
			seen[idx] = a.NodeName
		}
	}
	return nil
}

// GetValidatorIndices returns the validator indices for a given node name.
func (r *ValidatorRegistry) GetValidatorIndices(nodeName string) []uint64 {
	for _, a := range r.Assignments {
		if a.NodeName == nodeName {
			return a.Validators
		}
	}
	return nil
}
