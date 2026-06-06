package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func writeConfigYAML(outputDir string, genesisTime uint64, validators []validatorInfo) error {
	return writeOutput(outputDir, "config.yaml", renderConfigYAML(genesisTime, validators))
}

func writeAnnotatedValidatorsYAML(outputDir string, validators []validatorInfo, numNodes int) error {
	return writeOutput(outputDir, "annotated_validators.yaml", renderAnnotatedValidatorsYAML(validators, numNodes))
}

func writeNodesYAML(outputDir string, nodes []nodeInfo, basePort int) error {
	return writeOutput(outputDir, "nodes.yaml", renderNodesYAML(nodes, basePort))
}

func renderConfigYAML(genesisTime uint64, validators []validatorInfo) string {
	var out strings.Builder
	fmt.Fprintf(&out, "GENESIS_TIME: %d\nGENESIS_VALIDATORS:\n", genesisTime)
	for _, v := range validators {
		fmt.Fprintf(&out, "  - attestation_pubkey: \"%s\"\n    proposal_pubkey: \"%s\"\n",
			v.AttestationPubkeyHex, v.ProposalPubkeyHex)
	}
	return out.String()
}

func renderAnnotatedValidatorsYAML(validators []validatorInfo, numNodes int) string {
	if numNodes < 1 {
		return ""
	}
	nodeValidators := make(map[int][]validatorInfo)
	for _, v := range validators {
		nodeIdx := v.Index % numNodes
		nodeValidators[nodeIdx] = append(nodeValidators[nodeIdx], v)
	}

	var out strings.Builder
	for i := range numNodes {
		fmt.Fprintf(&out, "node%d:\n", i)
		for _, v := range nodeValidators[i] {
			fmt.Fprintf(&out, "  - index: %d\n    attestation_pubkey_hex: %s\n    proposal_pubkey_hex: %s\n    attestation_sk_file: %s\n    proposal_sk_file: %s\n",
				v.Index, v.AttestationPubkeyHex, v.ProposalPubkeyHex,
				v.AttestationSkFile, v.ProposalSkFile)
		}
	}
	return out.String()
}

func renderNodesYAML(nodes []nodeInfo, basePort int) string {
	var out strings.Builder
	for i, node := range nodes {
		port := basePort + i
		fmt.Fprintf(&out, "- \"/ip4/127.0.0.1/udp/%d/quic-v1/p2p/%s\"\n", port, node.PeerID)
	}
	return out.String()
}

func writeOutput(outputDir, name, data string) error {
	path := filepath.Join(outputDir, name)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
