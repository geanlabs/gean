package host

import (
	"context"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/geanlabs/gean/internal/core/lifecycle"
	"github.com/geanlabs/gean/internal/core/p2p/peers"
)

// HandleStream serves inbound streams for protocolID with handle, run as one
// of the host's tasks.
func (h *Host) HandleStream(protocolID string, handle network.StreamHandler) {
	h.host.SetStreamHandler(protocol.ID(protocolID), h.tracked(handle))
}

// tracked runs handle as one of the host's tasks, so Close waits for it. Once
// Close has begun, a new stream is reset without being handled.
func (h *Host) tracked(handle network.StreamHandler) network.StreamHandler {
	return func(s network.Stream) {
		if !h.tasks.Enter() {
			s.Reset()
			return
		}
		defer h.tasks.Leave()
		defer s.Close()
		handle(s)
	}
}

// NewStream opens a stream to id for protocolID.
func (h *Host) NewStream(ctx context.Context, id peer.ID, protocolID string) (network.Stream, error) {
	return h.host.NewStream(ctx, id, protocol.ID(protocolID))
}

// PeerStore returns the host's tracked peers.
func (h *Host) PeerStore() *peers.Store {
	return h.peerStore
}

// Tasks is the group Close joins. Work that uses the host, or state its caller
// frees after Close, runs in it.
func (h *Host) Tasks() *lifecycle.Group {
	return &h.tasks
}

// RecordRequest scores one outbound block request against its peer: an error is
// a failure, any response a success. A request cut short by our own
// cancellation says nothing about the peer.
func (h *Host) RecordRequest(parent context.Context, id peer.ID, err error) {
	if err != nil && parent.Err() != nil {
		return
	}
	outcome := "success"
	if err != nil {
		outcome = "failure"
	}
	OnRequestOutcome(outcome)
	h.peerStore.RecordRequest(id, err == nil)
}
