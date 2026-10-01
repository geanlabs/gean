//go:build tamago && riscv64 && zkvm_openvm

package goos

// OpenVM memory map (v2.x.0-preview.2, RV64): a 4 GiB address space with no
// fixed layout, but every pointer the VM dereferences must have its upper 32
// bits clear, and the Go toolchain sign-extends 32-bit constants at or above
// 0x8000_0000. Code sits at 0x1000_1000; Go data, bss, heap and the stack live
// in 1 GiB of RAM at 0x2000_0000, below the input region that zkio reserves
// at its end, and everything stays under 0x8000_0000.
var (
	RamStart       uint = 0x2000_0000
	RamSize        uint = 0x4000_0000
	RamStackOffset uint = 0x100
)

// printkByte is the buffer Printk passes to the print phantom instruction.
var printkByte byte
