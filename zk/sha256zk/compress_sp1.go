//go:build tamago && riscv64 && zkvm_sp1

package sha256

import (
	"encoding/binary"
	"unsafe"
)

// compress runs SP1's SHA_EXTEND and SHA_COMPRESS precompiles. SP1 v6 holds
// each 32-bit word of the message schedule and of the state in a 64-bit slot.
func compress(h *[4]uint64, block *[8]uint64) {
	b := (*[BlockSize]byte)(unsafe.Pointer(block))
	var w [64]uint64
	for i := range 16 {
		w[i] = uint64(binary.BigEndian.Uint32(b[4*i:]))
	}
	shaExtend(&w)
	st := (*[8]uint32)(unsafe.Pointer(h))
	var hw [8]uint64
	for i, v := range st {
		hw[i] = uint64(v)
	}
	shaCompress(&w, &hw)
	for i := range st {
		st[i] = uint32(hw[i])
	}
}

// defined in compress_sp1_riscv64.s
//
//go:noescape
func shaExtend(w *[64]uint64)

//go:noescape
func shaCompress(w *[64]uint64, h *[8]uint64)
