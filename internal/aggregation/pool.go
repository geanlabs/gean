package aggregation

import (
	"sync"

	"github.com/geanlabs/gean/xmss"
)

var (
	childProofsPool = sync.Pool{New: func() any { s := make([]xmss.ChildProof, 0, 8); return &s }}
	rawPubkeysPool  = sync.Pool{New: func() any { s := make([]xmss.CPubKey, 0, 32); return &s }}
	rawSigsPool     = sync.Pool{New: func() any { s := make([]xmss.CSig, 0, 32); return &s }}
	rawIDsPool      = sync.Pool{New: func() any { s := make([]uint64, 0, 32); return &s }}
)

func getChildProofsBuf() *[]xmss.ChildProof  { return childProofsPool.Get().(*[]xmss.ChildProof) }
func putChildProofsBuf(b *[]xmss.ChildProof) { *b = (*b)[:0]; childProofsPool.Put(b) }

func getRawPubkeysBuf() *[]xmss.CPubKey  { return rawPubkeysPool.Get().(*[]xmss.CPubKey) }
func putRawPubkeysBuf(b *[]xmss.CPubKey) { *b = (*b)[:0]; rawPubkeysPool.Put(b) }

func getRawSigsBuf() *[]xmss.CSig  { return rawSigsPool.Get().(*[]xmss.CSig) }
func putRawSigsBuf(b *[]xmss.CSig) { *b = (*b)[:0]; rawSigsPool.Put(b) }

func getRawIDsBuf() *[]uint64  { return rawIDsPool.Get().(*[]uint64) }
func putRawIDsBuf(b *[]uint64) { *b = (*b)[:0]; rawIDsPool.Put(b) }
