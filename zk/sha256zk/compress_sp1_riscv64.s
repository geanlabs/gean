//go:build tamago && riscv64 && zkvm_sp1

#include "textflag.h"

// SP1 syscalls: the number goes in T0, arguments in A0 and A1.
#define SHA_EXTEND	0x00300105
#define SHA_COMPRESS	0x00010106

// func shaExtend(w *[64]uint64)
TEXT ·shaExtend(SB),NOSPLIT,$0-8
	MOV	w+0(FP), A0
	MOV	$0, A1
	MOV	$SHA_EXTEND, T0
	ECALL
	RET

// func shaCompress(w *[64]uint64, h *[8]uint64)
TEXT ·shaCompress(SB),NOSPLIT,$0-16
	MOV	w+0(FP), A0
	MOV	h+8(FP), A1
	MOV	$SHA_COMPRESS, T0
	ECALL
	RET
