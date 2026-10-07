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

type ENRFields struct {
	Seq       uint64
	PeerID    peer.ID
	Multiaddr string
}

type enrRecord struct {
	seq       uint64
	peerID    peer.ID
	ip4       net.IP
	ip6       net.IP
	udpPort   uint16
	quicPort  uint16
	udp6Port  uint16
	quic6Port uint16
}

func DecodeENR(enrStr string) (*ENRFields, error) {
	rec, err := parseENRRecord(enrStr)
	if err != nil {
		return nil, err
	}
	return &ENRFields{
		Seq:       rec.seq,
		PeerID:    rec.peerID,
		Multiaddr: rec.transportMultiaddr(),
	}, nil
}

func ParseENR(enrStr string) (multiaddr.Multiaddr, error) {
	rec, err := parseENRRecord(enrStr)
	if err != nil {
		return nil, err
	}
	if rec.ip4 == nil && rec.ip6 == nil {
		return nil, fmt.Errorf("ENR missing ip/ip6 field")
	}

	transport := rec.transportMultiaddr()
	if transport == "" {
		return nil, fmt.Errorf("ENR missing udp/quic port")
	}

	ma, err := multiaddr.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", transport, rec.peerID))
	if err != nil {
		return nil, fmt.Errorf("build multiaddr: %w", err)
	}
	return ma, nil
}

func parseENRRecord(enrStr string) (*enrRecord, error) {
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

	var rec enrRecord
	if err := rlp.DecodeBytes(items[1], &rec.seq); err != nil {
		return nil, fmt.Errorf("decode ENR seq: %w", err)
	}

	var pubkeyBytes []byte
	for i := 2; i+1 < len(items); i += 2 {
		var key string
		if err := rlp.DecodeBytes(items[i], &key); err != nil {
			continue
		}
		switch key {
		case "ip":
			rec.ip4 = decodeIP(items[i+1], net.IPv4len)
		case "ip6":
			rec.ip6 = decodeIP(items[i+1], net.IPv6len)
		case "udp":
			rec.udpPort = decodePort(items[i+1])
		case "udp6":
			rec.udp6Port = decodePort(items[i+1])
		case "quic":
			rec.quicPort = decodePort(items[i+1])
		case "quic6":
			rec.quic6Port = decodePort(items[i+1])
		case "secp256k1":
			if err := rlp.DecodeBytes(items[i+1], &pubkeyBytes); err != nil {
				pubkeyBytes = nil
			}
		}
	}

	if len(pubkeyBytes) == 0 {
		return nil, fmt.Errorf("ENR missing secp256k1 key")
	}
	rec.peerID, err = peerIDFromSecp256k1(pubkeyBytes)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func decodeIP(raw rlp.RawValue, size int) net.IP {
	var ipBytes []byte
	if err := rlp.DecodeBytes(raw, &ipBytes); err != nil || len(ipBytes) != size {
		return nil
	}
	return net.IP(ipBytes)
}

func decodePort(raw rlp.RawValue) uint16 {
	var port uint16
	if err := rlp.DecodeBytes(raw, &port); err != nil {
		return 0
	}
	return port
}

func peerIDFromSecp256k1(pubkeyBytes []byte) (peer.ID, error) {
	secpKey, err := secp256k1.ParsePubKey(pubkeyBytes)
	if err != nil {
		return "", fmt.Errorf("parse secp256k1 key: %w", err)
	}

	libp2pKey, err := crypto.UnmarshalSecp256k1PublicKey(secpKey.SerializeCompressed())
	if err != nil {
		return "", fmt.Errorf("convert to libp2p key: %w", err)
	}

	peerID, err := peer.IDFromPublicKey(libp2pKey)
	if err != nil {
		return "", fmt.Errorf("derive peer ID: %w", err)
	}
	return peerID, nil
}

func (r *enrRecord) transportMultiaddr() string {
	if r.ip4 != nil {
		port := r.quicPort
		if port == 0 {
			port = r.udpPort
		}
		if port != 0 {
			return fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", r.ip4, port)
		}
	}
	if r.ip6 != nil {
		port := r.quic6Port
		if port == 0 {
			port = r.udp6Port
		}
		if port == 0 {
			port = r.quicPort
			if port == 0 {
				port = r.udpPort
			}
		}
		if port != 0 {
			return fmt.Sprintf("/ip6/%s/udp/%d/quic-v1", r.ip6, port)
		}
	}
	return ""
}

func buildTransportMultiaddr(ip4, ip6 net.IP, udpPort, quicPort, udp6Port, quic6Port uint16) string {
	return (&enrRecord{
		ip4:       ip4,
		ip6:       ip6,
		udpPort:   udpPort,
		quicPort:  quicPort,
		udp6Port:  udp6Port,
		quic6Port: quic6Port,
	}).transportMultiaddr()
}
