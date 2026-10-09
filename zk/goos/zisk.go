//go:build tamago && riscv64 && zkvm_zisk

package goos

// ZisK memory map (ziskos, v1.3): code and read-only data live in ROM at
// 0x8000_0000. RAM for the program starts after the reserved stack, register
// and output regions at 0xa043_0000 and ends where the float library's RAM
// begins at 0xbfff_0000. Go data and bss are linked at RamStart; the heap
// grows up from the end of bss towards the stack at the top.
var (
	RamStart       uint = 0xa043_0000
	RamSize        uint = 0xbfff_0000 - 0xa043_0000
	RamStackOffset uint = 0x100
)
