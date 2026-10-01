//go:build tamago && riscv64 && zkvm_sp1

package zkio

import (
	"crypto/sha256"
	"encoding/binary"
	"unsafe"
)

// SP1 input: the host passes the input record as one hint. The guest reads it
// into a reserved region past the end of RAM that nothing else touches, because
// SP1 only accepts hint writes to memory not yet written.
const (
	inputAddr = 0xc000_0000
	inputMax  = 0x1000_0000

	fdPublicValues = 13

	sysCommit               = 0x10
	sysCommitDeferredProofs = 0x1a
)

// committed holds every byte written to the public values, whose SHA-256
// digest Succeed commits.
var committed []byte

// ReadInput returns the input record in place, without copying.
func ReadInput() []byte {
	n := hintLen()
	if n == ^uint64(0) {
		return nil
	}
	if n > inputMax {
		Fail()
	}
	hintRead(inputAddr, uintptr(n))
	return unsafe.Slice((*byte)(unsafe.Pointer(uintptr(inputAddr))), n)
}

// Commit writes pv to the public values.
func Commit(pv []byte) {
	if len(pv) == 0 {
		return
	}
	write(fdPublicValues, unsafe.Pointer(&pv[0]), uintptr(len(pv)))
	committed = append(committed, pv...)
}

// Succeed commits the public values digest, commits no deferred proofs and
// halts with exit code 0.
func Succeed() {
	d := sha256.Sum256(committed)
	for i := range 8 {
		syscall2(sysCommit, uint64(i), uint64(binary.LittleEndian.Uint32(d[4*i:])))
	}
	for i := range 8 {
		syscall2(sysCommitDeferredProofs, uint64(i), 0)
	}
	halt()
}

// defined in sp1_riscv64.s
func hintLen() uint64
func hintRead(addr, n uintptr)
func write(fd uint64, p unsafe.Pointer, n uintptr)
func syscall2(code, a0, a1 uint64)
func halt()
func Fail()
