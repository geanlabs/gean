// Package discovery finds peers to dial: it decodes ENRs and reads bootnode
// lists.
package discovery

import (
	"crypto/ecdsa"
	"fmt"
	"net"
	"strings"

	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

type ENRFields struct {
	Seq       uint64
	PeerID    peer.ID
	Multiaddr string
	// Record is the decoded record, from which a chain reads its own entries.
	Record *enr.Record
}

type enrRecord struct {
	record    *enr.Record
	seq       uint64
	peerID    peer.ID
	ip4       net.IP
	ip6       net.IP
	udpPort   uint16
	quicPort  uint16
	udp6Port  uint16
	quic6Port uint16
}

// DecodeENR decodes a signed ENR text record; one whose signature does not
// verify is refused.
func DecodeENR(enrStr string) (*ENRFields, error) {
	rec, err := parseENRRecord(enrStr)
	if err != nil {
		return nil, err
	}
	return &ENRFields{
		Seq:       rec.seq,
		PeerID:    rec.peerID,
		Multiaddr: rec.transportMultiaddr(),
		Record:    rec.record,
	}, nil
}

// ParseENR returns the QUIC multiaddr of a signed ENR text record; one whose
// signature does not verify is refused.
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

	// enode.Parse verifies the signature: a record that fails it could name
	// any peer at any address.
	node, err := enode.Parse(enode.ValidSchemes, enrStr)
	if err != nil {
		return nil, fmt.Errorf("decode ENR: %w", err)
	}

	r := node.Record()
	rec := enrRecord{record: r, seq: r.Seq()}
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

	rec.peerID, err = peerIDFromPubkey(node.Pubkey())
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
