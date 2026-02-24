package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/geanlabs/gean/xmss/leansig"
)

func main() {
	count := flag.Int("validators", 5, "Number of keys to generate")
	outDir := flag.String("keys-dir", "keys", "Output directory for keys")
	printYAML := flag.Bool("print-yaml", false, "Print GENESIS_VALIDATORS yaml to stdout")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	var pubkeys []string

	fmt.Printf("Generating %d keys in %s...\n", *count, *outDir)
	for i := 0; i < *count; i++ {
		// Deterministic seed based on index
		seed := uint64(i)
		// Activation epoch 0, active for 256 epochs
		kp, err := leansig.GenerateKeypair(seed, 0, 256)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to generate keypair %d: %v\n", i, err)
			os.Exit(1)
		}
		defer kp.Free()

		pkPath := filepath.Join(*outDir, fmt.Sprintf("validator_%d_pk.ssz", i))
		skPath := filepath.Join(*outDir, fmt.Sprintf("validator_%d_sk.ssz", i))

		if err := leansig.SaveKeypair(kp, pkPath, skPath); err != nil {
			fmt.Fprintf(os.Stderr, "failed to save keypair %d: %v\n", i, err)
			os.Exit(1)
		}

		pkBytes, err := kp.PublicKeyBytes()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get public key bytes %d: %v\n", i, err)
			os.Exit(1)
		}
		pubkeys = append(pubkeys, hex.EncodeToString(pkBytes))

		fmt.Printf("Generated keypair %d\n", i)
	}

	if *printYAML {
		fmt.Println("\nGENESIS_VALIDATORS:")
		for _, pk := range pubkeys {
			fmt.Printf("  - \"0x%s\"\n", pk)
		}
	}
}
