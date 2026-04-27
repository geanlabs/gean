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

// ENRFields is the spec-compliant decoded view of an ENR per EIP-778.
// Use DecodeENR to obtain this. For dial-ready multiaddrs (with /p2p/<peerID>),
// use ParseENR instead — that form is what libp2p expects for outbound connects.
type ENRFields struct {
	// Seq is the ENR sequence number.
	Seq uint64

	// PeerID is the libp2p peer ID derived from the secp256k1 public key.
	PeerID peer.ID

	// Multiaddr is the transport-only address (no /p2p/<peerID> suffix).
	// Empty when the ENR carries neither an ip nor an ip6 field.
	Multiaddr string
}

// DecodeENR decodes an ENR to structured fields per EIP-778.
// Unlike ParseENR, this tolerates ENRs without an ip field (returns empty
// Multiaddr) and emits a transport-only multiaddr without /p2p/<peerID>.
func DecodeENR(enrStr string) (*ENRFields, error) {
	enrStr = strings.TrimSpace(enrStr)
	if !strings.HasPrefix(enrStr, "enr:") {
		return nil, fmt.Errorf("not an ENR: %q", enrStr)
	}

	data, err := base64.RawURLEncoding.DecodeString(enrStr[4:])
	if err != nil {
		return nil, fmt.Errorf("decode ENR base64: %w", err)
	}

	var items []rlp.RawValue
	if err := rlp.DecodeBytes(data, &items); err != nil {
		return nil, fmt.Errorf("decode ENR RLP: %w", err)
	}
	if len(items) < 4 {
		return nil, fmt.Errorf("ENR has too few items: %d", len(items))
	}

	var seq uint64
	if err := rlp.DecodeBytes(items[1], &seq); err != nil {
		return nil, fmt.Errorf("decode ENR seq: %w", err)
	}

	var ip4, ip6 net.IP
	var udpPort, quicPort uint16
	var udp6Port, quic6Port uint16
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
				ip4 = net.IP(ipBytes)
			}
		case "ip6":
			var ipBytes []byte
			if err := rlp.DecodeBytes(items[i+1], &ipBytes); err == nil && len(ipBytes) == 16 {
				ip6 = net.IP(ipBytes)
			}
		case "udp":
			var port uint16
			if err := rlp.DecodeBytes(items[i+1], &port); err == nil {
				udpPort = port
			}
		case "udp6":
			var port uint16
			if err := rlp.DecodeBytes(items[i+1], &port); err == nil {
				udp6Port = port
			}
		case "quic":
			var port uint16
			if err := rlp.DecodeBytes(items[i+1], &port); err == nil {
				quicPort = port
			}
		case "quic6":
			var port uint16
			if err := rlp.DecodeBytes(items[i+1], &port); err == nil {
				quic6Port = port
			}
		case "secp256k1":
			var raw []byte
			if err := rlp.DecodeBytes(items[i+1], &raw); err == nil {
				pubkeyBytes = raw
			}
		}
	}

	if len(pubkeyBytes) == 0 {
		return nil, fmt.Errorf("ENR missing secp256k1 key")
	}

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

	// Build transport-only multiaddr when possible. No /p2p/<peerID> suffix.
	// Prefer IPv4 when both are present (standard devp2p convention); fall back to IPv6.
	maStr := buildTransportMultiaddr(ip4, ip6, udpPort, quicPort, udp6Port, quic6Port)

	return &ENRFields{Seq: seq, PeerID: peerID, Multiaddr: maStr}, nil
}

// ParseENR decodes an ENR string and returns a dial-ready multiaddr.
// ENR format: "enr:<base64url-encoded RLP>"
// Prefers IPv4 when both ip and ip6 are present; falls back to IPv6.
// Emits /ip4/<addr>/udp/<port>/quic-v1/p2p/<peer_id> or /ip6/... depending on the address family.
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
	var ip4, ip6 net.IP
	var udpPort, quicPort uint16
	var udp6Port, quic6Port uint16
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
				ip4 = net.IP(ipBytes)
			}
		case "ip6":
			var ipBytes []byte
			if err := rlp.DecodeBytes(items[i+1], &ipBytes); err == nil && len(ipBytes) == 16 {
				ip6 = net.IP(ipBytes)
			}
		case "udp":
			var port uint16
			if err := rlp.DecodeBytes(items[i+1], &port); err == nil {
				udpPort = port
			}
		case "udp6":
			var port uint16
			if err := rlp.DecodeBytes(items[i+1], &port); err == nil {
				udp6Port = port
			}
		case "quic":
			var port uint16
			if err := rlp.DecodeBytes(items[i+1], &port); err == nil {
				quicPort = port
			}
		case "quic6":
			var port uint16
			if err := rlp.DecodeBytes(items[i+1], &port); err == nil {
				quic6Port = port
			}
		case "secp256k1":
			var raw []byte
			if err := rlp.DecodeBytes(items[i+1], &raw); err == nil {
				pubkeyBytes = raw
			}
		}
	}

	if ip4 == nil && ip6 == nil {
		return nil, fmt.Errorf("ENR missing ip/ip6 field")
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

	// Build transport multiaddr, then append /p2p/<peerID> for dialing.
	// Prefer IPv4 when both are present; fall back to IPv6.
	transport := buildTransportMultiaddr(ip4, ip6, udpPort, quicPort, udp6Port, quic6Port)
	if transport == "" {
		return nil, fmt.Errorf("ENR missing udp/quic port")
	}
	ma, err := multiaddr.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", transport, peerID))
	if err != nil {
		return nil, fmt.Errorf("build multiaddr: %w", err)
	}

	return ma, nil
}

// buildTransportMultiaddr emits a /ip4/.../udp/N/quic-v1 or /ip6/.../udp/N/quic-v1
// string from parsed ENR fields. Prefers IPv4 when both address families are present.
// Returns empty string when no usable (ip, port) pair is available.
func buildTransportMultiaddr(ip4, ip6 net.IP, udpPort, quicPort, udp6Port, quic6Port uint16) string {
	if ip4 != nil {
		port := quicPort
		if port == 0 {
			port = udpPort
		}
		if port != 0 {
			return fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", ip4, port)
		}
	}
	if ip6 != nil {
		port := quic6Port
		if port == 0 {
			port = udp6Port
		}
		if port == 0 {
			// Fallback: some ENRs reuse the ip4 quic/udp port for ip6.
			port = quicPort
			if port == 0 {
				port = udpPort
			}
		}
		if port != 0 {
			return fmt.Sprintf("/ip6/%s/udp/%d/quic-v1", ip6, port)
		}
	}
	return ""
}
