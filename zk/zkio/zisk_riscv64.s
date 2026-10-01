//go:build tamago && riscv64 && zkvm_zisk

#include "textflag.h"

// func inputReady(addr uintptr)
//
// fcall_input_ready: csrs 0x8F0, a0 passes the address as the call's single
// parameter, then csrwi 0x8C0, 23 invokes FCALL_INPUT_READY_ID.
TEXT ·inputReady(SB),NOSPLIT,$0-8
	MOV	addr+0(FP), A0
	WORD	$0x8F052073	// csrrs x0, 0x8F0, a0
	WORD	$0x8C0BD073	// csrrwi x0, 0x8C0, 23
	RET

// func Succeed()
TEXT ·Succeed(SB),NOSPLIT|NOFRAME,$0
	MOV	$0, A0
	MOV	$93, A7
	ECALL
loop:
	JMP	loop

// func Fail()
TEXT ·Fail(SB),NOSPLIT|NOFRAME,$0
	WORD	$0x00200073	// reserved: ZisK halts with an error, unprovable
loop:
	JMP	loop
