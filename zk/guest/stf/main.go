//go:build tamago && riscv64

// Command stf is the zkVM guest: it applies the input block to the input
// pre-state with gean's state transition and commits the public values.
// Any failure, including a runtime panic, halts without a proof.
package main

import (
	"runtime/debug"

	"github.com/geanlabs/gean/internal/zkstf"
	"github.com/geanlabs/gean/zk/zkio"
)

func main() {
	// Collection only costs cycles: one transition's garbage fits in RAM.
	debug.SetGCPercent(-1)

	pv, err := zkstf.Apply(zkio.ReadInput())
	if err != nil {
		zkio.Fail()
	}
	b := pv.Bytes()
	zkio.Commit(b[:])
	zkio.Succeed()
}
