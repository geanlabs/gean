package reqresp

import (
	"github.com/libp2p/go-libp2p/core/network"

	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/core/metrics"
)

// WriteResponse arms the idle write deadline before every response frame so a peer
// that stops reading cannot hold an inbound handler open indefinitely.
func WriteResponse(s network.Stream, label string, code byte, data []byte) bool {
	ArmWriteDeadline(s)
	encoded := EncodeResponse(code, data)
	metrics.ObserveReqRespResponseChunkSize(label, len(encoded))
	if _, err := s.Write(encoded); err != nil {
		if IsStreamTimeout(err) {
			metrics.IncReqRespTimeout(label, "write")
		}
		logger.Warn(logger.Network, "%s: write response failed: %v", label, err)
		return false
	}
	return true
}
