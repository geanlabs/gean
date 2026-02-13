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

// GetValidatorIndices returns the validator indices for a given node name.
func (r *ValidatorRegistry) GetValidatorIndices(nodeName string) []uint64 {
	for _, a := range r.Assignments {
		if a.NodeName == nodeName {
			return a.Validators
		}
	}
	return nil
}
