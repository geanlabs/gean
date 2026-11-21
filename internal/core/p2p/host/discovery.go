package host

import (
	"fmt"
	"maps"
	"net"
	"slices"
	"strconv"

	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/multiformats/go-multiaddr"

	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/core/p2p/discovery"
)

// StartDiscovery runs discv5 on udpPort with the host's key, advertising its
// QUIC address (at advertiseIP, when set) and the entries set so far, and
// dials the peers it finds until Close.
func (h *Host) StartDiscovery(udpPort int, advertiseIP net.IP, bootnodes []*enode.Node) error {
	raw, err := h.host.Peerstore().PrivKey(h.host.ID()).Raw()
	if err != nil {
		return fmt.Errorf("discv5 key: %w", err)
	}
	key, err := gethcrypto.ToECDSA(raw)
	if err != nil {
		return fmt.Errorf("discv5 key: %w", err)
	}
	quicPort, err := h.quicPort()
	if err != nil {
		return err
	}

	h.enrMu.Lock()
	defer h.enrMu.Unlock()
	svc, err := discovery.Start(discovery.Config{
		PrivateKey:  key,
		Port:        udpPort,
		AdvertiseIP: advertiseIP,
		QUICPort:    quicPort,
		Bootnodes:   bootnodes,
		Entries:     slices.Collect(maps.Values(h.enrEntries)),
	})
	if err != nil {
		return err
	}
	h.discovery, h.enrEntries = svc, nil
	h.tasks.Go(func() { svc.Dial(h.ctx, h.discoveryAdmit, h.ConnectedPeers, h.ConnectPeer) })
	logger.Info(logger.Network, "discv5 started bootnodes=%d enr=%s", len(bootnodes), svc.Self())
	return nil
}

// Node returns this host's discv5 record, or nil when discovery is not running.
func (h *Host) Node() *enode.Node {
	h.enrMu.Lock()
	defer h.enrMu.Unlock()
	if h.discovery == nil {
		return nil
	}
	return h.discovery.Self()
}

// SetENREntry puts entry in this host's discv5 record, or holds it for the
// record StartDiscovery creates.
func (h *Host) SetENREntry(entry enr.Entry) {
	h.enrMu.Lock()
	defer h.enrMu.Unlock()
	if h.discovery != nil {
		h.discovery.Set(entry)
		return
	}
	if h.enrEntries == nil {
		h.enrEntries = make(map[string]enr.Entry)
	}
	h.enrEntries[entry.ENRKey()] = entry
}

// DeleteENREntry is the inverse of SetENREntry.
func (h *Host) DeleteENREntry(entry enr.Entry) {
	h.enrMu.Lock()
	defer h.enrMu.Unlock()
	if h.discovery != nil {
		h.discovery.Delete(entry)
		return
	}
	delete(h.enrEntries, entry.ENRKey())
}

// quicPort returns the UDP port the host's QUIC listener is bound to.
func (h *Host) quicPort() (uint16, error) {
	for _, addr := range h.host.Network().ListenAddresses() {
		if _, err := addr.ValueForProtocol(multiaddr.P_QUIC_V1); err != nil {
			continue
		}
		port, err := addr.ValueForProtocol(multiaddr.P_UDP)
		if err != nil {
			continue
		}
		n, err := strconv.ParseUint(port, 10, 16)
		return uint16(n), err
	}
	return 0, fmt.Errorf("host has no QUIC listener")
}
