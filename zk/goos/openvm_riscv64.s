//go:build tamago && riscv64 && zkvm_openvm

#include "textflag.h"

// OpenVM custom instructions use the custom-0 opcode (0x0b) in I-type form:
// imm[11:0]<<20 | rs1<<15 | funct3<<12 | rd<<7 | 0x0b.

TEXT ·CPUInit(SB),NOSPLIT|NOFRAME,$0
	MOV	·RamStart(SB), X2
	MOV	·RamSize(SB), T0
	MOV	·RamStackOffset(SB), T1
	ADD	T0, X2
	SUB	T1, X2
	JMP	_rt0_tamago_start(SB)

TEXT ·Hwinit0(SB),NOSPLIT|NOFRAME,$0
	RET

// func Nanotime() int64
TEXT ·Nanotime(SB),NOSPLIT,$0-8
	MOV	·clock(SB), T0
	ADD	$1000, T0
	MOV	T0, ·clock(SB)
	MOV	T0, ret+0(FP)
	RET

// func Printk(c byte)
//
// Prints one byte with the print_str phantom instruction (funct3 3, imm 1,
// rd = pointer, rs1 = length), which the executor writes to stdout.
TEXT ·Printk(SB),NOSPLIT,$0-1
	MOVBU	c+0(FP), T0
	MOVB	T0, ·printkByte(SB)
	MOV	$·printkByte(SB), A0
	MOV	$1, A1
	WORD	$0x0015b50b	// print_str a0, a1
	RET

// func fail(code int32)
//
// terminate(1): OpenVM ends the run with exit code 1, which neither an app
// proof nor the aggregation circuit accepts, so no proof exists for it. The
// runtime's exit code is deliberately not used.
TEXT ·fail(SB),NOSPLIT|NOFRAME,$0-4
	WORD	$0x0010000b
loop:
	JMP	loop
