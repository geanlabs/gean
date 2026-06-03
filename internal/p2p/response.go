package p2p

import (
	"io"

	"github.com/geanlabs/gean/internal/logger"
)

func writeResponse(w io.Writer, label string, code byte, data []byte) bool {
	if _, err := w.Write(EncodeResponse(code, data)); err != nil {
		logger.Warn(logger.Network, "%s: write response failed: %v", label, err)
		return false
	}
	return true
}
