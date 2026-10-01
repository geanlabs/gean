//go:build tamago && riscv64 && zkvm_zisk

package zkio

import "unsafe"

// ZisK input: a free-input word at 0x4000_0000, then the u64 length of the
// first input record and its bytes. The host must make each byte available
// (input_ready) before the guest reads it, or the proof cannot be built.
const (
	inputLenAddr  = 0x4000_0008
	inputDataAddr = 0x4000_0010
	outputAddr    = 0xa041_0000
	outputSlots   = 64
)

// ReadInput returns the input record in place, without copying.
func ReadInput() []byte {
	inputReady(inputLenAddr + 7)
	n := *(*uint64)(unsafe.Pointer(uintptr(inputLenAddr)))
	if n == 0 {
		return nil
	}
	inputReady(inputDataAddr + uintptr(n) - 1)
	return unsafe.Slice((*byte)(unsafe.Pointer(uintptr(inputDataAddr))), n)
}

// Commit writes pv into the public output slots as little-endian u32 words.
func Commit(pv []byte) {
	if len(pv) > outputSlots*4 {
		Fail()
	}
	for i := 0; i < len(pv); i += 4 {
		var w uint32
		for j := 0; j < 4 && i+j < len(pv); j++ {
			w |= uint32(pv[i+j]) << (8 * j)
		}
		*(*uint32)(unsafe.Pointer(uintptr(outputAddr + i))) = w
	}
}

// defined in zisk_riscv64.s
func inputReady(addr uintptr)
func Succeed()
func Fail()
