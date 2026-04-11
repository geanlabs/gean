package main

// Keygen generates all config files needed to run a standalone gean devnet.
//
// First run: generates XMSS keys, node keys, and all config files (~40s per validator).
// Subsequent runs: skips key generation, only refreshes config.yaml with new genesis time.
//
// Usage:
//   go run ./cmd/keygen --validators 5 --nodes 3 --output testnet

// #cgo linux LDFLAGS: -L${SRCDIR}/../../xmss/rust/target/release -lhashsig_glue -lmultisig_glue -lm -ldl -lpthread
// #cgo darwin LDFLAGS: -L${SRCDIR}/../../xmss/rust/target/release -lhashsig_glue -lmultisig_glue -lm -ldl -lpthread -framework CoreFoundation -framework SystemConfiguration -framework Security
// #include <stdint.h>
// #include <stdlib.h>
// typedef struct KeyPair KeyPair;
// typedef struct PublicKey PublicKey;
// typedef struct PrivateKey PrivateKey;
//
// KeyPair* hashsig_keypair_generate(const char* seed_phrase,
//     size_t activation_epoch, size_t num_active_epochs);
// void hashsig_keypair_free(KeyPair* keypair);
// const PublicKey* hashsig_keypair_get_public_key(const KeyPair* keypair);
// const PrivateKey* hashsig_keypair_get_private_key(const KeyPair* keypair);
// size_t hashsig_public_key_to_bytes(const PublicKey* public_key, uint8_t* buffer, size_t buffer_len);
// size_t hashsig_private_key_to_bytes(const PrivateKey* private_key, uint8_t* buffer, size_t buffer_len);
import "C"

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/types"
)

// manifest stores generated key info so we can skip regeneration.
type manifest struct {
	Validators []validatorInfo `json:"validators"`
	Nodes      []nodeInfo      `json:"nodes"`
}

type validatorInfo struct {
	Index                int    `json:"index"`
	AttestationPubkeyHex string `json:"attestation_pubkey_hex"`
	ProposalPubkeyHex    string `json:"proposal_pubkey_hex"`
	AttestationSkFile    string `json:"attestation_sk_file"`
	ProposalSkFile       string `json:"proposal_sk_file"`
}

type nodeInfo struct {
	KeyFile string `json:"key_file"`
	PeerID  string `json:"peer_id"`
}

func main() {
	numValidators := flag.Int("validators", 5, "Number of validators to generate")
	numNodes := flag.Int("nodes", 3, "Number of nodes")
	outputDir := flag.String("output", "testnet", "Output directory")
	basePort := flag.Int("base-port", 9000, "Base P2P port (incremented per node)")

	flag.Parse()

	if *numValidators < 1 || *numNodes < 1 {
		log.Fatal("need at least 1 validator and 1 node")
	}

	os.MkdirAll(*outputDir, 0755)
	keysDir := filepath.Join(*outputDir, "hash-sig-keys")
	os.MkdirAll(keysDir, 0755)

	manifestPath := filepath.Join(*outputDir, "manifest.json")

	// Try to load existing manifest (skip key generation if valid).
	var m manifest
	if existing, err := loadManifest(manifestPath); err == nil &&
		len(existing.Validators) == *numValidators &&
		len(existing.Nodes) == *numNodes &&
		keysExist(keysDir, existing.Validators) &&
		nodeKeysExist(*outputDir, existing.Nodes) {

		log.Printf("keys already exist (%d validators, %d nodes) — skipping generation",
			len(existing.Validators), len(existing.Nodes))
		m = *existing
	} else {
		// Generate fresh keys.
		m = generateKeys(*numValidators, *numNodes, *outputDir, keysDir, *basePort)
		saveManifest(manifestPath, &m)
	}

	// Always refresh config.yaml with fresh genesis time (30 seconds from now).
	genesisTime := uint64(time.Now().Unix()) + 30
	writeConfigYAML(*outputDir, genesisTime, m.Validators)
	writeAnnotatedValidatorsYAML(*outputDir, m.Validators, *numNodes)
	writeNodesYAML(*outputDir, m.Nodes, *basePort)

	log.Println("---")
	log.Printf("output: %s", *outputDir)
	log.Printf("genesis time: %d (in 30 seconds: %s)", genesisTime,
		time.Unix(int64(genesisTime), 0).Format(time.RFC3339))
	log.Printf("validators: %d, nodes: %d", len(m.Validators), len(m.Nodes))
	log.Println("")
	log.Println("run immediately:")
	log.Printf("  bin/gean --custom-network-config-dir %s --node-key %s/node0.key --node-id node0 --is-aggregator --data-dir data/node0",
		*outputDir, *outputDir)
}

func generateKeys(numValidators, numNodes int, outputDir, keysDir string, basePort int) manifest {
	var m manifest

	// Generate dual XMSS validator keys (attestation + proposal per validator).
	log.Printf("generating %d XMSS validator keypairs (2 keys each, ~40s per key)...", numValidators)
	for i := 0; i < numValidators; i++ {
		log.Printf("  generating validator %d/%d...", i+1, numValidators)

		attPubHex, attSkFile := generateAndSaveKey(i, "attestation", keysDir)
		propPubHex, propSkFile := generateAndSaveKey(i, "proposal", keysDir)

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

	// Generate node keys.
	log.Printf("generating %d node keys...", numNodes)
	for i := 0; i < numNodes; i++ {
		keyBytes := make([]byte, 32)
		rand.Read(keyBytes)
		keyHex := hex.EncodeToString(keyBytes)

		keyFile := fmt.Sprintf("node%d.key", i)
		keyPath := filepath.Join(outputDir, keyFile)
		os.WriteFile(keyPath, []byte(keyHex), 0600)

		privKey, _ := libp2pcrypto.UnmarshalSecp256k1PrivateKey(keyBytes)
		peerID, _ := peer.IDFromPrivateKey(privKey)

		m.Nodes = append(m.Nodes, nodeInfo{
			KeyFile: keyFile,
			PeerID:  peerID.String(),
		})
		log.Printf("  node%d: peer_id=%s", i, peerID)
	}

	return m
}

// generateAndSaveKey generates one XMSS keypair and saves the SK file.
// Returns (pubkey_hex, sk_filename).
func generateAndSaveKey(validatorIdx int, keyType, keysDir string) (string, string) {
	seed := fmt.Sprintf("gean-testnet-validator-%d-%s-%d", validatorIdx, keyType, time.Now().UnixNano())

	cSeed := C.CString(seed)
	kp := C.hashsig_keypair_generate(cSeed, C.size_t(0), C.size_t(1<<18))
	C.free(unsafe.Pointer(cSeed))
	if kp == nil {
		log.Fatalf("key generation failed for validator %d %s", validatorIdx, keyType)
	}

	var pkBuf [256]byte
	pkLen := C.hashsig_public_key_to_bytes(
		C.hashsig_keypair_get_public_key(kp),
		(*C.uint8_t)(unsafe.Pointer(&pkBuf[0])),
		C.size_t(len(pkBuf)),
	)
	if pkLen == 0 || int(pkLen) != types.PubkeySize {
		log.Fatalf("pubkey serialization failed for validator %d %s", validatorIdx, keyType)
	}

	skBuf := make([]byte, 10*1024*1024)
	skLen := C.hashsig_private_key_to_bytes(
		C.hashsig_keypair_get_private_key(kp),
		(*C.uint8_t)(unsafe.Pointer(&skBuf[0])),
		C.size_t(len(skBuf)),
	)
	if skLen == 0 {
		log.Fatalf("private key serialization failed for validator %d %s", validatorIdx, keyType)
	}

	C.hashsig_keypair_free(kp)

	skFile := fmt.Sprintf("validator_%d_%s_sk.ssz", validatorIdx, keyType)
	skPath := filepath.Join(keysDir, skFile)
	os.WriteFile(skPath, skBuf[:skLen], 0600)

	return hex.EncodeToString(pkBuf[:pkLen]), skFile
}

func writeConfigYAML(outputDir string, genesisTime uint64, validators []validatorInfo) {
	y := fmt.Sprintf("GENESIS_TIME: %d\nGENESIS_VALIDATORS:\n", genesisTime)
	for _, v := range validators {
		y += fmt.Sprintf("  - attestation_pubkey: \"%s\"\n    proposal_pubkey: \"%s\"\n",
			v.AttestationPubkeyHex, v.ProposalPubkeyHex)
	}
	os.WriteFile(filepath.Join(outputDir, "config.yaml"), []byte(y), 0644)
}

func writeAnnotatedValidatorsYAML(outputDir string, validators []validatorInfo, numNodes int) {
	nodeValidators := make(map[int][]validatorInfo)
	for _, v := range validators {
		nodeIdx := v.Index % numNodes
		nodeValidators[nodeIdx] = append(nodeValidators[nodeIdx], v)
	}
	y := ""
	for i := 0; i < numNodes; i++ {
		y += fmt.Sprintf("node%d:\n", i)
		for _, v := range nodeValidators[i] {
			y += fmt.Sprintf("  - index: %d\n    attestation_pubkey_hex: %s\n    proposal_pubkey_hex: %s\n    attestation_sk_file: %s\n    proposal_sk_file: %s\n",
				v.Index, v.AttestationPubkeyHex, v.ProposalPubkeyHex,
				v.AttestationSkFile, v.ProposalSkFile)
		}
	}
	os.WriteFile(filepath.Join(outputDir, "annotated_validators.yaml"), []byte(y), 0644)
}

func writeNodesYAML(outputDir string, nodes []nodeInfo, basePort int) {
	yaml := ""
	for i, node := range nodes {
		port := basePort + i
		yaml += fmt.Sprintf("- \"/ip4/127.0.0.1/udp/%d/quic-v1/p2p/%s\"\n", port, node.PeerID)
	}
	os.WriteFile(filepath.Join(outputDir, "nodes.yaml"), []byte(yaml), 0644)
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

func saveManifest(path string, m *manifest) {
	data, _ := json.MarshalIndent(m, "", "  ")
	os.WriteFile(path, data, 0644)
}

func keysExist(keysDir string, validators []validatorInfo) bool {
	for _, v := range validators {
		if _, err := os.Stat(filepath.Join(keysDir, v.AttestationSkFile)); err != nil {
			return false
		}
		if _, err := os.Stat(filepath.Join(keysDir, v.ProposalSkFile)); err != nil {
			return false
		}
	}
	return true
}

func nodeKeysExist(outputDir string, nodes []nodeInfo) bool {
	for _, n := range nodes {
		if _, err := os.Stat(filepath.Join(outputDir, n.KeyFile)); err != nil {
			return false
		}
	}
	return true
}
