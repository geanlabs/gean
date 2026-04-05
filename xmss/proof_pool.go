package xmss

import "sync"

// proofBufPool reuses 1 MiB proof serialization buffers to reduce GC pressure.
var proofBufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, MaxProofSize)
		return &buf
	},
}

// getProofBuf returns a 1 MiB buffer from the pool.
func getProofBuf() *[]byte {
	return proofBufPool.Get().(*[]byte)
}

// putProofBuf returns a buffer to the pool.
func putProofBuf(buf *[]byte) {
	proofBufPool.Put(buf)
}
