//go:build tamago && riscv64 && zkvm_zisk

#include "textflag.h"

// UART: a single-byte store is echoed to the emulator's stdout.
#define UART_ADDR 0xa0400200

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
TEXT ·Printk(SB),NOSPLIT,$0-1
	MOVBU	c+0(FP), T0
	MOV	$UART_ADDR, T1
	MOVB	T0, (T1)
	RET

// func fail(code int32)
//
// ZisK's exit ecall discards the exit code, so failure must not use it. The
// SYSTEM word 0x00200073 decodes as a reserved instruction, which ZisK turns
// into halt_with_error: the run aborts and no proof can be produced.
TEXT ·fail(SB),NOSPLIT|NOFRAME,$0-4
	WORD	$0x00200073
loop:
	JMP	loop
