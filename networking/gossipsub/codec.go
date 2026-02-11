// codec.go contains snappy codec helpers used by gossip publish/consume paths.
package gossipsub

import "github.com/golang/snappy"

// CompressMessage compresses data using snappy for gossipsub.
func CompressMessage(data []byte) []byte {
	return snappy.Encode(nil, data)
}

// DecompressMessage decompresses snappy-compressed data.
func DecompressMessage(data []byte) ([]byte, error) {
	return snappy.Decode(nil, data)
}
