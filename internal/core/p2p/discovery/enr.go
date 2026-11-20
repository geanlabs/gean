// Package discovery finds peers to dial: it decodes ENRs and reads bootnode
// lists.
package discovery

import (
	"crypto/ecdsa"
	"encoding/base64"
	"fmt"
	"net"
	"strings"

	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
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

	// The signature is left unchecked (no enode.New): leanSpec's fixtures
	// expect records with invalid signatures to decode.
	var r enr.Record
	if err := rlp.DecodeBytes(data, &r); err != nil {
		return nil, fmt.Errorf("decode ENR RLP: %w", err)
	}

	rec := enrRecord{seq: r.Seq()}
	var ip4 enr.IPv4
	if r.Load(&ip4) == nil {
		rec.ip4 = net.IP(ip4)
	}
	var ip6 enr.IPv6
	if r.Load(&ip6) == nil {
		rec.ip6 = net.IP(ip6)
	}
	// A port entry that is missing or malformed stays zero, which
	// transportMultiaddr reads as absent.
	var udp enr.UDP
	var udp6 enr.UDP6
	var quic enr.QUIC
	var quic6 enr.QUIC6
	for _, e := range []enr.Entry{&udp, &udp6, &quic, &quic6} {
		_ = r.Load(e)
	}
	rec.udpPort, rec.udp6Port = uint16(udp), uint16(udp6)
	rec.quicPort, rec.quic6Port = uint16(quic), uint16(quic6)

	var pub enode.Secp256k1
	if err := r.Load(&pub); err != nil {
		return nil, fmt.Errorf("ENR secp256k1 key: %w", err)
	}
	rec.peerID, err = peerIDFromPubkey((*ecdsa.PublicKey)(&pub))
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func peerIDFromPubkey(pub *ecdsa.PublicKey) (peer.ID, error) {
	key, err := crypto.UnmarshalSecp256k1PublicKey(gethcrypto.CompressPubkey(pub))
	if err != nil {
		return "", fmt.Errorf("convert to libp2p key: %w", err)
	}
	peerID, err := peer.IDFromPublicKey(key)
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
