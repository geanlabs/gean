package p2p

import (
	"github.com/libp2p/go-libp2p/core/network"

	"github.com/geanlabs/gean/logger"
)

// writeResponse arms the idle write deadline before every response frame so a peer
// that stops reading cannot hold an inbound handler open indefinitely.
func (h *Host) writeResponse(s network.Stream, label string, code byte, data []byte) bool {
	armWriteDeadline(s)
	encoded := EncodeResponse(code, data)
	if h.Hooks.ReqRespResponseChunkSize != nil {
		h.Hooks.ReqRespResponseChunkSize(label, len(encoded))
	}
	if _, err := s.Write(encoded); err != nil {
		if isStreamTimeout(err) {
			if h.Hooks.ReqRespTimeout != nil {
				h.Hooks.ReqRespTimeout(label, "write")
			}
		}
		logger.Warn(logger.Network, "%s: write response failed: %v", label, err)
		return false
	}
	return true
}
