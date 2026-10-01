//go:build tamago && riscv64 && zkvm_openvm

package zkio

import (
	"encoding/binary"
	"unsafe"
)

// OpenVM input: the host passes the input record as one stdin vector. The
// guest loads it into the hint stream, reads its u64 length, then copies it in
// dwords into a reserved region past the end of RAM that nothing else touches.
const (
	inputAddr = 0x6000_0000
	inputMax  = 0x1000_0000

	// maxHintDwords is the most dwords one hint_buffer may write.
	maxHintDwords = 1023

	// publicValuesSize is the VM's public values size in bytes: a multiple
	// of 8 whose dword count is a power of two, so the 96-byte record is
	// revealed zero-padded to 128.
	publicValuesSize = 128
)

// committed holds the public values that Succeed reveals.
var committed [publicValuesSize]byte

var ncommitted int

// hintLen is where hint_stored writes the input length; it must be 8-byte
// aligned.
var hintLen uint64

// ReadInput returns the input record in place, without copying.
func ReadInput() []byte {
	hintInput()
	hintStored(uintptr(unsafe.Pointer(&hintLen)))
	n := hintLen
	if n > inputMax {
		Fail()
	}
	for off := uint64(0); off < n; off += 8 * maxHintDwords {
		dwords := min((n-off+7)/8, maxHintDwords)
		hintBuffer(uintptr(inputAddr+off), uintptr(dwords))
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(uintptr(inputAddr))), n)
}

// Commit appends pv to the public values.
func Commit(pv []byte) {
	if len(pv) > publicValuesSize-ncommitted {
		Fail()
	}
	ncommitted += copy(committed[ncommitted:], pv)
}

// Succeed reveals every public values dword, zero padding included, and
// terminates with exit code 0.
func Succeed() {
	for i := 0; i < publicValuesSize; i += 8 {
		reveal(uint64(i), binary.LittleEndian.Uint64(committed[i:]))
	}
	terminate()
}

// defined in openvm_riscv64.s
func hintInput()
func hintStored(p uintptr)
func hintBuffer(p, dwords uintptr)
func reveal(offset, v uint64)
func terminate()
func Fail()
