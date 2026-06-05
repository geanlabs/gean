package xmss

import "sync"

var proofBufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, MaxProofSize)
		return &buf
	},
}

func getProofBuf() *[]byte {
	return proofBufPool.Get().(*[]byte)
}

func putProofBuf(buf *[]byte) {
	proofBufPool.Put(buf)
}
