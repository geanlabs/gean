// Command haltcheck runs the haltcheck guest through each termination path on
// a zkVM's local executor and checks that only a committed run halts
// successfully.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/geanlabs/gean/internal/zkstf"
	"github.com/geanlabs/gean/internal/zkstf/ziskemu"
	"github.com/geanlabs/gean/internal/zkstf/zkhost"
)

func main() {
	zkvm := flag.String("zkvm", "zisk", "zkvm: zisk (runs $ZISKEMU), sp1 or openvm (runs $<ZKVM>HOST)")
	elf := flag.String("elf", "", "haltcheck guest ELF")
	flag.Parse()
	var execute func(context.Context, []byte) (*zkstf.ExecResult, error)
	switch *zkvm {
	case "zisk":
		execute = ziskemu.Executor{Bin: envOr("ZISKEMU", "ziskemu"), ELF: *elf}.Execute
	case "sp1", "openvm":
		host := envOr(strings.ToUpper(*zkvm)+"HOST", *zkvm+"host")
		execute = zkhost.Prover{VM: zkstf.ZKVM(*zkvm), Bin: host, ELF: *elf}.Execute
	default:
		log.Fatalf("unsupported zkvm %q", *zkvm)
	}

	failed := false
	for _, mode := range []string{"commit", "return", "panic", "nil"} {
		_, err := execute(context.Background(), []byte(mode))
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

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
