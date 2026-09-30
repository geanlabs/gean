// Command haltcheck runs the haltcheck guest through each termination path on
// the ZisK emulator and checks that only a committed run halts successfully.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/geanlabs/gean/internal/zkstf"
	"github.com/geanlabs/gean/internal/zkstf/ziskemu"
)

func main() {
	elf := flag.String("elf", "", "haltcheck guest ELF")
	flag.Parse()
	bin := os.Getenv("ZISKEMU")
	if bin == "" {
		bin = "ziskemu"
	}
	emu := ziskemu.Prover{Bin: bin, ELF: *elf}

	failed := false
	for _, mode := range []string{"commit", "return", "panic", "nil"} {
		_, err := emu.Execute(context.Background(), []byte(mode))
		wantOK := mode == "commit"
		ok := err == nil
		switch {
		case ok == wantOK:
			fmt.Printf("%-7s ok\n", mode)
		case err != nil && !errors.Is(err, zkstf.ErrGuestFailed):
			log.Fatalf("%s: %v", mode, err)
		default:
			fmt.Printf("%-7s WRONG: success=%v err=%v\n", mode, ok, err)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}
