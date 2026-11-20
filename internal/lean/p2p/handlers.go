package p2p

import (
	"github.com/libp2p/go-libp2p/core/network"

	"github.com/geanlabs/gean/internal/lean/types"
)

func (h *Host) RegisterReqRespHandlers(
	statusFn func() *StatusMessage,
	blockByRootFn func(root [32]byte) *types.SignedBlock,
	currentSlotFn func() uint64,
	blocksInRangeFn func(startSlot, count uint64) ([]*types.SignedBlock, bool),
) {
	h.HandleStream(StatusProtocol, func(s network.Stream) {
		if peerStatus := handleStatusRequest(s, statusFn); peerStatus != nil {
			h.PeerStore().UpdateStatus(s.Conn().RemotePeer(), peerStatus.HeadSlot)
		}
	})

	h.HandleStream(BlocksByRootProtocol, func(s network.Stream) {
		handleBlocksByRootRequest(s, blockByRootFn)
	})

	h.HandleStream(BlocksByRangeProtocol, func(s network.Stream) {
		handleBlocksByRangeRequest(s, currentSlotFn, blocksInRangeFn)
	})
}
