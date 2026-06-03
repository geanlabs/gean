package p2p

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
)

func loadNodeKey(path string) (libp2pcrypto.PrivKey, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	hexStr := strings.TrimPrefix(strings.TrimSpace(string(content)), "0x")
	keyBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("decode hex key: %w", err)
	}

	privKey, err := libp2pcrypto.UnmarshalSecp256k1PrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse secp256k1 key: %w", err)
	}
	return privKey, nil
}
