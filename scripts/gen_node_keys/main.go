package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"os"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"gopkg.in/yaml.v3"
)

type bootnodeEntry struct {
	Multiaddr string `yaml:"multiaddr"`
}

func main() {
	nodes := flag.Int("nodes", 3, "Number of node keys to generate")
	ip := flag.String("ip", "127.0.0.1", "IP to embed in generated multiaddrs")
	basePort := flag.Int("base-port", 9000, "Base TCP port for node multiaddrs (nodeN uses base-port+N)")
	outPath := flag.String("out", "nodes.yaml", "Output path for nodes.yaml")
	flag.Parse()

	entries := make([]bootnodeEntry, 0, *nodes)

	for i := 0; i < *nodes; i++ {
		filename := fmt.Sprintf("node%d.key", i)

		priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to generate secp256k1 key for node%d: %v\n", i, err)
			os.Exit(1)
		}

		bytes, err := crypto.MarshalPrivateKey(priv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal private key for node%d: %v\n", i, err)
			os.Exit(1)
		}

		if err := os.WriteFile(filename, bytes, 0600); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", filename, err)
			os.Exit(1)
		}

		pid, err := peer.IDFromPrivateKey(priv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to derive peer ID for node%d: %v\n", i, err)
			os.Exit(1)
		}

		addr := fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", *ip, *basePort+i, pid.String())
		entries = append(entries, bootnodeEntry{Multiaddr: addr})
		fmt.Fprintf(os.Stderr, "node%d: %s\n", i, pid.String())
	}

	yamlBytes, err := yaml.Marshal(entries)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal nodes.yaml: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*outPath, yamlBytes, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", *outPath, err)
		os.Exit(1)
	}
}
