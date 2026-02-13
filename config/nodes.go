package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// BootnodeConfig represents a bootnode entry from nodes.yaml.
type BootnodeConfig struct {
	Name      string `yaml:"name"`
	Multiaddr string `yaml:"multiaddr"`
}

// LoadBootnodes loads and parses a nodes.yaml file.
func LoadBootnodes(path string) ([]BootnodeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read nodes: %w", err)
	}

	var nodes []BootnodeConfig
	if err := yaml.Unmarshal(data, &nodes); err != nil {
		return nil, fmt.Errorf("parse nodes: %w", err)
	}

	return nodes, nil
}
