package main

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/geanlabs/gean/internal/types"
)

func loadOrGenerate(opts options, keysDir, manifestPath string) (manifest, bool, error) {
	if existing, err := loadManifest(manifestPath); err == nil && manifestUsable(existing, opts, keysDir) {
		return *existing, true, nil
	}

	m, err := generateKeys(opts, keysDir)
	if err != nil {
		return manifest{}, false, err
	}
	if err := saveManifest(manifestPath, &m); err != nil {
		return manifest{}, false, err
	}
	return m, false, nil
}

func manifestUsable(m *manifest, opts options, keysDir string) bool {
	return m != nil &&
		validatorsUsable(m.Validators, opts.Validators, keysDir) &&
		nodesUsable(m.Nodes, opts.Nodes, opts.OutputDir)
}

func loadManifest(path string) (*manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func saveManifest(path string, m *manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	return nil
}

func validatorsUsable(validators []validatorInfo, want int, keysDir string) bool {
	if len(validators) != want {
		return false
	}
	seen := make(map[int]bool, len(validators))
	for _, v := range validators {
		if v.Index < 0 || v.Index >= want || seen[v.Index] {
			return false
		}
		seen[v.Index] = true
		if !validPubkeyHex(v.AttestationPubkeyHex) || !validPubkeyHex(v.ProposalPubkeyHex) {
			return false
		}
		if v.AttestationSkFile == "" || v.ProposalSkFile == "" {
			return false
		}
		if !fileExists(filepath.Join(keysDir, v.AttestationSkFile)) ||
			!fileExists(filepath.Join(keysDir, v.ProposalSkFile)) {
			return false
		}
	}
	return true
}

func nodesUsable(nodes []nodeInfo, want int, outputDir string) bool {
	if len(nodes) != want {
		return false
	}
	seenKeys := make(map[string]bool, len(nodes))
	seenPeers := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		if n.KeyFile == "" || n.PeerID == "" || seenKeys[n.KeyFile] || seenPeers[n.PeerID] {
			return false
		}
		seenKeys[n.KeyFile] = true
		seenPeers[n.PeerID] = true
		if !fileExists(filepath.Join(outputDir, n.KeyFile)) {
			return false
		}
	}
	return true
}

func validPubkeyHex(s string) bool {
	if len(s) != types.PubkeySize*2 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
