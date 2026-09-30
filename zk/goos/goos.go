//go:build tamago && riscv64

// Package goos is the TamaGo runtime overlay (GOOSPKG) that runs gean's
// state-transition guest bare-metal inside a zkVM.
//
// A zkVM has no clock, no interrupts, no entropy and a single hart. Time is a
// counter that only moves forward, random data is a fixed deterministic
// stream, and every way the runtime can terminate — a panic, a fatal error, or
// main returning — ends in the zkVM's failing halt. The only successful exit is
// zkio.Succeed, after the public values are committed.
package goos

import "unsafe"

const (
	ArenaBaseOffset     = 0
	HeapAddrBits        = 40
	LogHeapArenaBytes   = 2 + 20
	LogPallocChunkPages = 9
	MinPhysPageSize     = 4096
	StackSystem         = 0
)

var (
	Bloc   uintptr
	Exit   = fail
	Idle   = idle
	ProcID func() uint64
	Task   func(sp, mp, gp, fn unsafe.Pointer)
	Wake   func(procid uint64)
)

// clock is the fake monotonic time, in nanoseconds, returned by Nanotime.
var clock int64

// idle fast-forwards the fake clock to the next timer instead of spinning.
func idle(until int64) {
	if until > clock {
		clock = until
	}
}

func Hwinit1() {}

func InitRNG() {}

// GetRandomData fills b from a fixed xorshift stream. The runtime only uses it
// to seed hash maps; a deterministic seed keeps executions reproducible.
func GetRandomData(b []byte) {
	for i := range b {
		rng ^= rng << 13
		rng ^= rng >> 7
		rng ^= rng << 17
		b[i] = byte(rng)
	}
}

var rng uint64 = 0x9e3779b97f4a7c15

// defined in the per-zkVM assembly
func CPUInit()
func Hwinit0()
func Nanotime() int64
func Printk(c byte)
func fail(code int32)
