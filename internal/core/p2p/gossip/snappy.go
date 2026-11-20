package gossip

import (
	"fmt"

	"github.com/golang/snappy"

	"github.com/geanlabs/gean/internal/core/p2p/reqresp"
)

// Compress encodes a gossip payload in the snappy block format.
func Compress(data []byte) []byte {
	return snappy.Encode(nil, data)
}

// Decompress decodes a snappy block-format gossip payload, refusing one that
// would decompress past reqresp.MaxPayloadSize.
func Decompress(data []byte) ([]byte, error) {
	decodedLen, err := snappy.DecodedLen(data)
	if err != nil {
		return nil, fmt.Errorf("snappy decoded len: %w", err)
	}
	if decodedLen > reqresp.MaxPayloadSize {
		return nil, fmt.Errorf("snappy decoded len %d exceeds max %d", decodedLen, reqresp.MaxPayloadSize)
	}
	return snappy.Decode(nil, data)
}
