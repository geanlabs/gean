package ssz

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/devlongs/gean/common/types"
)

const BytesPerChunk = 32

var ZeroHash = types.Root{}

func Hash(data []byte) types.Root {
	return types.Root(sha256.Sum256(data))
}

func HashNodes(a, b types.Root) types.Root {
	h := sha256.New()
	h.Write(a[:])
	h.Write(b[:])
	var result types.Root
	copy(result[:], h.Sum(nil))
	return result
}

func HashTreeRootUint64(value uint64) types.Root {
	var buf [32]byte
	binary.LittleEndian.PutUint64(buf[:8], value)
	return types.Root(buf)
}

func Merkleize(chunks []types.Root, limit int) types.Root {
	n := len(chunks)

	if n == 0 {
		if limit > 0 {
			return zeroTreeRoot(nextPowerOfTwo(limit))
		}
		return ZeroHash
	}

	width := nextPowerOfTwo(n)
	if limit > 0 && limit >= n {
		width = nextPowerOfTwo(limit)
	}

	if width == 1 {
		return chunks[0]
	}

	level := make([]types.Root, width)
	copy(level, chunks)

	for len(level) > 1 {
		next := make([]types.Root, len(level)/2)
		for i := range next {
			next[i] = HashNodes(level[i*2], level[i*2+1])
		}
		level = next
	}

	return level[0]
}

func MixInLength(root types.Root, length uint64) types.Root {
	var lenChunk types.Root
	binary.LittleEndian.PutUint64(lenChunk[:8], length)
	return HashNodes(root, lenChunk)
}

func nextPowerOfTwo(x int) int {
	if x <= 1 {
		return 1
	}
	n := x - 1
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	return n + 1
}

func zeroTreeRoot(width int) types.Root {
	if width <= 1 {
		return ZeroHash
	}
	h := ZeroHash
	for width > 1 {
		h = HashNodes(h, h)
		width /= 2
	}
	return h
}
