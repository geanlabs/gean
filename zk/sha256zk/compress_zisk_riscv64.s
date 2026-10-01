//go:build tamago && riscv64 && zkvm_zisk

#include "textflag.h"

// func sha256f(p *sha256fParams)
//
// ZisK precompile call: csrs 0x805 (SYSCALL_SHA256F_ID), a0.
TEXT ·sha256f(SB),NOSPLIT,$0-8
	MOV	p+0(FP), A0
	WORD	$0x80552073	// csrrs x0, 0x805, a0
	RET
