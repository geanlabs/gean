//go:build tamago && riscv64 && zkvm_sp1

#include "textflag.h"

// SP1 syscalls: the number goes in T0, arguments in A0 and A1 (and A2 for
// write), and a result comes back in T0.

// func hintLen() uint64
TEXT ·hintLen(SB),NOSPLIT,$0-8
	MOV	$0xF0, T0
	ECALL
	MOV	T0, ret+0(FP)
	RET

// func hintRead(addr, n uintptr)
TEXT ·hintRead(SB),NOSPLIT,$0-16
	MOV	addr+0(FP), A0
	MOV	n+8(FP), A1
	MOV	$0xF1, T0
	ECALL
	RET

// func write(fd uint64, p unsafe.Pointer, n uintptr)
TEXT ·write(SB),NOSPLIT,$0-24
	MOV	fd+0(FP), A0
	MOV	p+8(FP), A1
	MOV	n+16(FP), A2
	MOV	$0x02, T0
	ECALL
	RET

// func syscall2(code, a0, a1 uint64)
TEXT ·syscall2(SB),NOSPLIT,$0-24
	MOV	code+0(FP), T0
	MOV	a0+8(FP), A0
	MOV	a1+16(FP), A1
	ECALL
	RET

// func halt()
TEXT ·halt(SB),NOSPLIT|NOFRAME,$0
	MOV	$0, A0
	MOV	$0x00, T0
	ECALL
loop:
	JMP	loop

// func Fail()
TEXT ·Fail(SB),NOSPLIT|NOFRAME,$0
	WORD	$0xc0001073	// UNIMP: SP1 aborts the run, unprovable
loop:
	JMP	loop
