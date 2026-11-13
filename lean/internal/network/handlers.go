package network

import (
	libp2pnetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/geanlabs/gean/lean/consensus/types"
)

func (h *Host) RegisterReqRespHandlers(
	statusFn func() *StatusMessage,
	blockByRootFn func(root [32]byte) *types.SignedBlock,
	currentSlotFn func() uint64,
	blocksInRangeFn func(startSlot, count uint64) ([]*types.SignedBlock, bool),
) {
	h.host.SetStreamHandler(protocol.ID(StatusProtocol), h.tracked(func(s libp2pnetwork.Stream) {
		if peerStatus := handleStatusRequest(s, statusFn); peerStatus != nil {
			h.peerStore.UpdateStatus(s.Conn().RemotePeer(), peerStatus.HeadSlot)
		}
	}))

	h.host.SetStreamHandler(protocol.ID(BlocksByRootProtocol), h.tracked(func(s libp2pnetwork.Stream) {
		handleBlocksByRootRequest(s, blockByRootFn)
	}))

	h.host.SetStreamHandler(protocol.ID(BlocksByRangeProtocol), h.tracked(func(s libp2pnetwork.Stream) {
		handleBlocksByRangeRequest(s, currentSlotFn, blocksInRangeFn)
	}))
}

// tracked runs handle as one of the host's tasks, so Close waits for it. Once
// Close has begun, a new stream is reset without being handled.
func (h *Host) tracked(handle libp2pnetwork.StreamHandler) libp2pnetwork.StreamHandler {
	return func(s libp2pnetwork.Stream) {
		if !h.tasks.Enter() {
			s.Reset()
			return
		}
		defer h.tasks.Leave()
		defer s.Close()
		handle(s)
	}
}
