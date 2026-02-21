package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// bootnodeEntry represents a bootnode with named fields (legacy format).
type bootnodeEntry struct {
	Multiaddr string `yaml:"multiaddr"`
}

// LoadBootnodes loads a nodes.yaml file and returns raw bootnode strings.
// Supports both formats:
//   - Legacy:  [{multiaddr: "/ip4/..."}]
//   - ENR:     ["enr:-IW4Q..."]
func LoadBootnodes(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read nodes: %w", err)
	}

	// Try legacy struct format first.
	var entries []bootnodeEntry
	if err := yaml.Unmarshal(data, &entries); err == nil && len(entries) > 0 && entries[0].Multiaddr != "" {
		out := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.Multiaddr != "" {
				out = append(out, e.Multiaddr)
			}
		}
		return out, nil
	}

	// Fall back to plain string list (ENR or multiaddr strings).
	var strs []string
	if err := yaml.Unmarshal(data, &strs); err != nil {
		return nil, fmt.Errorf("parse nodes: %w", err)
	}
	return strs, nil
}
