package reqresp

import (
	"github.com/libp2p/go-libp2p/core/network"

	"github.com/geanlabs/gean/internal/core/logger"
)

// WriteResponse arms the idle write deadline before every response frame so a peer
// that stops reading cannot hold an inbound handler open indefinitely.
func WriteResponse(s network.Stream, label string, code byte, data []byte) bool {
	ArmWriteDeadline(s)
	encoded := EncodeResponse(code, data)
	OnResponseChunk(label, len(encoded))
	if _, err := s.Write(encoded); err != nil {
		if IsStreamTimeout(err) {
			OnTimeout(label, "write")
		}
		logger.Warn(logger.Network, "%s: write response failed: %v", label, err)
		return false
	}
	return true
}
