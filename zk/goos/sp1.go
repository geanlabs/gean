//go:build tamago && riscv64 && zkvm_sp1

package goos

// SP1 memory map (v6): every loaded segment must lie at or above 0x7800_0000,
// where code starts. Go data, bss, heap and the stack live in 1 GiB of RAM at
// 0x8000_0000, within PC-relative reach of the code, and below the input
// region that zkio reserves at its end.
var (
	RamStart       uint = 0x8000_0000
	RamSize        uint = 0x4000_0000
	RamStackOffset uint = 0x100
)

// printkByte is the buffer Printk passes to the write syscall.
var printkByte byte
