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
// would decompress past reqresp.MaxPayloadSize or whose header declares more
// than its bytes can decode to.
func Decompress(data []byte) ([]byte, error) {
	decodedLen, err := snappy.DecodedLen(data)
	if err != nil {
		return nil, fmt.Errorf("snappy decoded len: %w", err)
	}
	// snappy.Decode allocates the declared length up front, so the header must
	// not be trusted past what the bytes can hold. A literal emits no more bytes
	// than it occupies, and the densest element, a copy with a 2-byte offset,
	// emits at most 64 bytes from 3, so a block expands at most 64/3 times.
	// Format: https://github.com/google/snappy/blob/main/format_description.txt
	if limit := min(reqresp.MaxPayloadSize, len(data)*64/3); decodedLen > limit {
		return nil, fmt.Errorf("snappy decoded len %d exceeds limit %d for %d bytes", decodedLen, limit, len(data))
	}
	return snappy.Decode(nil, data)
}
