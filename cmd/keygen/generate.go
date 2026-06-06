package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/geanlabs/gean/xmss"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func generateKeys(opts options, keysDir string) (manifest, error) {
	var m manifest

	log.Printf("generating %d XMSS validator keypairs (2 keys each, ~40s per key)...", opts.Validators)
	for i := range opts.Validators {
		log.Printf("  generating validator %d/%d...", i+1, opts.Validators)

		attPubHex, attSkFile, err := generateAndSaveKey(i, "attestation", keysDir)
		if err != nil {
			return manifest{}, err
		}
		propPubHex, propSkFile, err := generateAndSaveKey(i, "proposal", keysDir)
		if err != nil {
			return manifest{}, err
		}

		m.Validators = append(m.Validators, validatorInfo{
			Index:                i,
			AttestationPubkeyHex: attPubHex,
			ProposalPubkeyHex:    propPubHex,
			AttestationSkFile:    attSkFile,
			ProposalSkFile:       propSkFile,
		})

		log.Printf("  validator %d: att=%s...%s prop=%s...%s",
			i, attPubHex[:8], attPubHex[len(attPubHex)-8:],
			propPubHex[:8], propPubHex[len(propPubHex)-8:])
	}

	log.Printf("generating %d node keys...", opts.Nodes)
	for i := range opts.Nodes {
		node, err := generateNodeKey(opts.OutputDir, i)
		if err != nil {
			return manifest{}, err
		}
		m.Nodes = append(m.Nodes, node)
		log.Printf("  node%d: peer_id=%s", i, node.PeerID)
	}

	return m, nil
}

func generateAndSaveKey(validatorIdx int, keyType, keysDir string) (string, string, error) {
	seed := fmt.Sprintf("gean-testnet-validator-%d-%s-%d", validatorIdx, keyType, time.Now().UnixNano())

	kp, err := xmss.GenerateKeyPair(seed, 0, 1<<18)
	if err != nil {
		return "", "", fmt.Errorf("generate %s key for validator %d: %w", keyType, validatorIdx, err)
	}
	defer kp.Close()

	pkBytes, err := kp.PublicKeyBytes()
	if err != nil {
		return "", "", fmt.Errorf("serialize %s pubkey for validator %d: %w", keyType, validatorIdx, err)
	}

	skBytes, err := kp.PrivateKeyBytes()
	if err != nil {
		return "", "", fmt.Errorf("serialize %s secret key for validator %d: %w", keyType, validatorIdx, err)
	}

	skFile := fmt.Sprintf("validator_%d_%s_sk.ssz", validatorIdx, keyType)
	skPath := filepath.Join(keysDir, skFile)
	if err := os.WriteFile(skPath, skBytes, 0o600); err != nil {
		return "", "", fmt.Errorf("write %s secret key for validator %d: %w", keyType, validatorIdx, err)
	}

	return hex.EncodeToString(pkBytes[:]), skFile, nil
}

func generateNodeKey(outputDir string, index int) (nodeInfo, error) {
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return nodeInfo{}, fmt.Errorf("generate node%d key: %w", index, err)
	}

	privKey, err := libp2pcrypto.UnmarshalSecp256k1PrivateKey(keyBytes)
	if err != nil {
		return nodeInfo{}, fmt.Errorf("parse node%d key: %w", index, err)
	}
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nodeInfo{}, fmt.Errorf("derive node%d peer id: %w", index, err)
	}

	keyFile := fmt.Sprintf("node%d.key", index)
	keyPath := filepath.Join(outputDir, keyFile)
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(keyBytes)), 0o600); err != nil {
		return nodeInfo{}, fmt.Errorf("write node%d key: %w", index, err)
	}

	return nodeInfo{KeyFile: keyFile, PeerID: peerID.String()}, nil
}
