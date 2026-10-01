//go:build tamago && riscv64 && zkvm_openvm

#include "textflag.h"

// OpenVM custom-0 instructions (opcode 0x0b), I-type:
// imm[11:0]<<20 | rs1<<15 | funct3<<12 | rd<<7 | 0x0b.

// func hintInput()
//
// hint_input phantom (funct3 3, imm 0): resets the hint stream to the next
// stdin vector as its u64 length, its bytes and zero padding to 8 bytes.
TEXT ·hintInput(SB),NOSPLIT,$0
	WORD	$0x0000300b
	RET

// func hintStored(p uintptr)
//
// hint_stored (funct3 1, imm 0): writes the next 8 hint bytes to [rd].
TEXT ·hintStored(SB),NOSPLIT,$0-8
	MOV	p+0(FP), A0
	WORD	$0x0000150b	// hint_stored a0
	RET

// func hintBuffer(p, dwords uintptr)
//
// hint_buffer (funct3 1, imm 1): writes rs1 dwords of hint to [rd].
TEXT ·hintBuffer(SB),NOSPLIT,$0-16
	MOV	p+0(FP), A0
	MOV	dwords+8(FP), A1
	WORD	$0x0015950b	// hint_buffer a0, a1
	RET

// func reveal(offset, v uint64)
//
// reveal (funct3 2, imm 0): writes the dword rs1 to the public values at
// byte offset [rd].
TEXT ·reveal(SB),NOSPLIT,$0-16
	MOV	offset+0(FP), A0
	MOV	v+8(FP), A1
	WORD	$0x0005a50b	// reveal a1 at a0
	RET

// func terminate()
//
// terminate(0) (funct3 0, imm = exit code): the only successful halt.
TEXT ·terminate(SB),NOSPLIT|NOFRAME,$0
	WORD	$0x0000000b
loop:
	JMP	loop

// func Fail()
TEXT ·Fail(SB),NOSPLIT|NOFRAME,$0
	WORD	$0x0010000b	// terminate(1): not provable
loop:
	JMP	loop
