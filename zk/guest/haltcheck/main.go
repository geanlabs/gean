//go:build tamago && riscv64

// Command haltcheck exercises a zkVM board's termination paths. The first
// input byte picks one; only "commit" may end in a successful halt.
package main

import "github.com/geanlabs/gean/zk/zkio"

func main() {
	in := zkio.ReadInput()
	switch string(in) {
	case "commit":
		var pv [96]byte // a public values record: three roots
		copy(pv[:], "ok")
		zkio.Commit(pv[:])
		zkio.Succeed()
	case "panic":
		panic("haltcheck")
	case "nil":
		var p *int
		_ = *p
	}
	// "return": main returns without committing; the runtime exits and
	// the board must halt as a failure.
}
