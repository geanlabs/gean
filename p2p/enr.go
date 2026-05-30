package p2p

import (
	"encoding/base64"
	"fmt"
	"net"
	"strings"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	leanrlp "github.com/geanlabs/gean/internal/rlp"
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

type decodedENR struct {
	seq       uint64
	peerID    peer.ID
	ip4       net.IP
	ip6       net.IP
	udpPort   uint16
	quicPort  uint16
	udp6Port  uint16
	quic6Port uint16
}

func (d *decodedENR) transportMultiaddr() string {
	return buildTransportMultiaddr(d.ip4, d.ip6, d.udpPort, d.quicPort, d.udp6Port, d.quic6Port)
}

// DecodeENR decodes an ENR to structured fields per EIP-778.
// Unlike ParseENR, this tolerates ENRs without an ip field (returns empty
// Multiaddr) and emits a transport-only multiaddr without /p2p/<peerID>.
func DecodeENR(enrStr string) (*ENRFields, error) {
	decoded, err := decodeENR(enrStr)
	if err != nil {
		return nil, err
	}

	return &ENRFields{
		Seq:       decoded.seq,
		PeerID:    decoded.peerID,
		Multiaddr: decoded.transportMultiaddr(),
	}, nil
}

func decodeENR(enrStr string) (*decodedENR, error) {
	enrStr = strings.TrimSpace(enrStr)
	if !strings.HasPrefix(enrStr, "enr:") {
		return nil, fmt.Errorf("not an ENR: %q", enrStr)
	}

	data, err := base64.RawURLEncoding.DecodeString(enrStr[4:])
	if err != nil {
		return nil, fmt.Errorf("decode ENR base64: %w", err)
	}

	items, err := leanrlp.SplitList(data)
	if err != nil {
		return nil, fmt.Errorf("decode ENR RLP: %w", err)
	}
	if len(items) < 4 {
		return nil, fmt.Errorf("ENR has too few items: %d", len(items))
	}

	seq, err := leanrlp.Uint64(items[1])
	if err != nil {
		return nil, fmt.Errorf("decode ENR seq: %w", err)
	}

	decoded := &decodedENR{seq: seq}
	var pubkeyBytes []byte

	for i := 2; i+1 < len(items); i += 2 {
		key, err := leanrlp.String(items[i])
		if err != nil {
			continue
		}
		switch key {
		case "ip":
			if ipBytes, err := leanrlp.Bytes(items[i+1]); err == nil && len(ipBytes) == 4 {
				decoded.ip4 = net.IP(ipBytes)
			}
		case "ip6":
			if ipBytes, err := leanrlp.Bytes(items[i+1]); err == nil && len(ipBytes) == 16 {
				decoded.ip6 = net.IP(ipBytes)
			}
		case "udp":
			if port, err := leanrlp.Uint16(items[i+1]); err == nil {
				decoded.udpPort = port
			}
		case "udp6":
			if port, err := leanrlp.Uint16(items[i+1]); err == nil {
				decoded.udp6Port = port
			}
		case "quic":
			if port, err := leanrlp.Uint16(items[i+1]); err == nil {
				decoded.quicPort = port
			}
		case "quic6":
			if port, err := leanrlp.Uint16(items[i+1]); err == nil {
				decoded.quic6Port = port
			}
		case "secp256k1":
			if raw, err := leanrlp.Bytes(items[i+1]); err == nil {
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
	decoded.peerID = peerID

	return decoded, nil
}

// ParseENR decodes an ENR string and returns a dial-ready multiaddr.
// ENR format: "enr:<base64url-encoded RLP>"
// Prefers IPv4 when both ip and ip6 are present; falls back to IPv6.
// Emits /ip4/<addr>/udp/<port>/quic-v1/p2p/<peer_id> or /ip6/... depending on the address family.
func ParseENR(enrStr string) (multiaddr.Multiaddr, error) {
	decoded, err := decodeENR(enrStr)
	if err != nil {
		return nil, err
	}
	if decoded.ip4 == nil && decoded.ip6 == nil {
		return nil, fmt.Errorf("ENR missing ip/ip6 field")
	}

	// Build transport multiaddr, then append /p2p/<peerID> for dialing.
	// Prefer IPv4 when both are present; fall back to IPv6.
	transport := decoded.transportMultiaddr()
	if transport == "" {
		return nil, fmt.Errorf("ENR missing udp/quic port")
	}
	ma, err := multiaddr.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", transport, decoded.peerID))
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
