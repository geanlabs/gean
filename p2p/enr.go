package p2p

import (
	"encoding/base64"
	"fmt"
	"net"
	"strings"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// ParseENR decodes an ENR string and returns a multiaddr.
// ENR format: "enr:<base64url-encoded RLP>"
// Extracts ip, udp/quic port, and secp256k1 pubkey to build:
//
//	/ip4/{ip}/udp/{port}/quic-v1/p2p/{peer_id}
func ParseENR(enrStr string) (multiaddr.Multiaddr, error) {
	enrStr = strings.TrimSpace(enrStr)
	if !strings.HasPrefix(enrStr, "enr:") {
		return nil, fmt.Errorf("not an ENR: %q", enrStr)
	}

	b64 := enrStr[4:]
	data, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("decode ENR base64: %w", err)
	}

	// Decode RLP list: [signature, seq, k1, v1, k2, v2, ...]
	var items []rlp.RawValue
	if err := rlp.DecodeBytes(data, &items); err != nil {
		return nil, fmt.Errorf("decode ENR RLP: %w", err)
	}

	if len(items) < 4 {
		return nil, fmt.Errorf("ENR has too few items: %d", len(items))
	}

	// Parse key-value pairs (skip signature at index 0, seq at index 1).
	var ip net.IP
	var udpPort, quicPort uint16
	var pubkeyBytes []byte

	for i := 2; i+1 < len(items); i += 2 {
		var key string
		if err := rlp.DecodeBytes(items[i], &key); err != nil {
			continue
		}

		switch key {
		case "ip":
			var ipBytes []byte
			if err := rlp.DecodeBytes(items[i+1], &ipBytes); err == nil && len(ipBytes) == 4 {
				ip = net.IP(ipBytes)
			}
		case "udp":
			var port uint16
			if err := rlp.DecodeBytes(items[i+1], &port); err == nil {
				udpPort = port
			}
		case "quic":
			var port uint16
			if err := rlp.DecodeBytes(items[i+1], &port); err == nil {
				quicPort = port
			}
		case "secp256k1":
			var raw []byte
			if err := rlp.DecodeBytes(items[i+1], &raw); err == nil {
				pubkeyBytes = raw
			}
		}
	}

	if ip == nil {
		return nil, fmt.Errorf("ENR missing ip field")
	}

	port := quicPort
	if port == 0 {
		port = udpPort
	}
	if port == 0 {
		return nil, fmt.Errorf("ENR missing udp/quic port")
	}

	if len(pubkeyBytes) == 0 {
		return nil, fmt.Errorf("ENR missing secp256k1 key")
	}

	// Derive libp2p peer ID from secp256k1 compressed pubkey.
	secpKey, err := secp256k1.ParsePubKey(pubkeyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse secp256k1 key: %w", err)
	}

	libp2pKey, err := crypto.UnmarshalSecp256k1PublicKey(secpKey.SerializeCompressed())
	if err != nil {
		return nil, fmt.Errorf("convert to libp2p key: %w", err)
	}

	peerID, err := peer.IDFromPublicKey(libp2pKey)
	if err != nil {
		return nil, fmt.Errorf("derive peer ID: %w", err)
	}

	// Build multiaddr.
	maStr := fmt.Sprintf("/ip4/%s/udp/%d/quic-v1/p2p/%s", ip, port, peerID)
	ma, err := multiaddr.NewMultiaddr(maStr)
	if err != nil {
		return nil, fmt.Errorf("build multiaddr: %w", err)
	}

	return ma, nil
}
