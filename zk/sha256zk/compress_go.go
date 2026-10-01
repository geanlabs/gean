//go:build !(tamago && riscv64 && (zkvm_zisk || zkvm_sp1 || zkvm_openvm))

package sha256

import "unsafe"

func compress(h *[4]uint64, block *[8]uint64) {
	compressGo((*[8]uint32)(unsafe.Pointer(h)), (*[BlockSize]byte)(unsafe.Pointer(block)))
}
