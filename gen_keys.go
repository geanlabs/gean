package main

import (
	"crypto/rand"
	"fmt"
	"os"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func main() {
	for i := 0; i < 3; i++ {
		filename := fmt.Sprintf("node%d.key", i)
		// Check if key already exists to key stable IDs if re-run
		if _, err := os.Stat(filename); err == nil {
			// Load existing
			bytes, err := os.ReadFile(filename)
			if err != nil {
				panic(err)
			}
			priv, err := crypto.UnmarshalPrivateKey(bytes)
			if err != nil {
				panic(err)
			}
			pid, err := peer.IDFromPrivateKey(priv)
			if err != nil {
				panic(err)
			}
			fmt.Printf("node%d: %s\n", i, pid.String())
			continue
		}

		// Generate new
		priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
		if err != nil {
			panic(err)
		}

		bytes, err := crypto.MarshalPrivateKey(priv)
		if err != nil {
			panic(err)
		}

		err = os.WriteFile(filename, bytes, 0600)
		if err != nil {
			panic(err)
		}

		pid, err := peer.IDFromPrivateKey(priv)
		if err != nil {
			panic(err)
		}

		fmt.Printf("node%d: %s\n", i, pid.String())
	}
}
