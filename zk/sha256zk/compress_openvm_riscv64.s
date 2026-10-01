//go:build tamago && riscv64 && zkvm_openvm

#include "textflag.h"

// func compress(h *[4]uint64, block *[8]uint64)
//
// OpenVM SHA-256 compress, custom-0 R-type: funct7 2, funct3 4, rd = new
// state, rs1 = previous state, rs2 = block.
TEXT ·compress(SB),NOSPLIT,$0-16
	MOV	h+0(FP), A0
	MOV	block+8(FP), A1
	WORD	$0x04b5450b	// sha256 a0 <- a0, a1
	RET
