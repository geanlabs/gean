//go:build tamago && riscv64 && zkvm_sp1

#include "textflag.h"

// SP1 syscall numbers, passed in T0.
#define SYS_WRITE 0x02

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
// Writes one byte to the executor's stdout (fd 1).
TEXT ·Printk(SB),NOSPLIT,$0-1
	MOVBU	c+0(FP), T0
	MOVB	T0, ·printkByte(SB)
	MOV	$1, A0
	MOV	$·printkByte(SB), A1
	MOV	$1, A2
	MOV	$SYS_WRITE, T0
	ECALL
	RET

// func fail(code int32)
//
// UNIMP (csrrw x0, cycle, x0) transpiles to SP1's unimplemented opcode:
// executing it aborts the run, so no proof exists for it. The exit code is
// deliberately not used; SP1 can prove a HALT with any code.
TEXT ·fail(SB),NOSPLIT|NOFRAME,$0-4
	WORD	$0xc0001073
loop:
	JMP	loop
