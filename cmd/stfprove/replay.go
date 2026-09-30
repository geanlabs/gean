package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/geanlabs/gean/internal/zkstf/replay"
)

const defaultNodeURL = "http://127.0.0.1:5052"

func runReplay(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: stfprove replay --zkvm <zkvm> [flags] block <id> | blocks <from-slot> <to-slot>")
		fs.PrintDefaults()
	}
	var backend backendFlags
	backend.register(fs)
	nodeURL := fs.String("node-url", envOr("STFPROVE_NODE_URL", defaultNodeURL), "node API base URL (env STFPROVE_NODE_URL)")
	out := fs.String("out", "", "write each block's input and public values under this directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ids, rangeMode, err := replayIDs(fs.Args())
	if err != nil {
		fs.Usage()
		return err
	}
	prover, err := backend.prover()
	if err != nil {
		return err
	}

	var (
		count   int
		cycles  uint64
		elapsed time.Duration
	)
	err = replay.Run(ctx, replay.NewClient(*nodeURL), prover, ids, rangeMode, func(b replay.Block) error {
		pv := b.Result.PublicValues
		if *out != "" {
			dir := filepath.Join(*out, fmt.Sprintf("slot-%d", b.Slot))
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, "in.bin"), b.Input, 0o644); err != nil {
				return err
			}
			enc := pv.Bytes()
			if err := os.WriteFile(filepath.Join(dir, "pv.bin"), enc[:], 0o644); err != nil {
				return err
			}
		}
		fmt.Fprintf(stdout, "slot=%d block=0x%x attestations=%d cycles=%d time=%s ok\n",
			b.Slot, pv.BlockRoot[:6], b.Attestations, b.Result.Cycles, b.Result.Duration.Round(time.Microsecond))
		count++
		cycles += b.Result.Cycles
		elapsed += b.Result.Duration
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s: replayed %d blocks, cycles=%d time=%s, all roots chain\n",
		prover.ZKVM(), count, cycles, elapsed.Round(time.Microsecond))
	return nil
}

// replayIDs turns "block <id>" or "blocks <from> <to>" into block ids; the
// range form skips empty slots.
func replayIDs(args []string) (ids []string, rangeMode bool, err error) {
	switch {
	case len(args) == 2 && args[0] == "block":
		return []string{args[1]}, false, nil
	case len(args) == 3 && args[0] == "blocks":
		from, err1 := strconv.ParseUint(args[1], 10, 64)
		to, err2 := strconv.ParseUint(args[2], 10, 64)
		if err1 != nil || err2 != nil || from == 0 || to < from {
			return nil, false, fmt.Errorf("%w: blocks needs slots 1 <= from <= to", errUsage)
		}
		for slot := from; slot <= to; slot++ {
			ids = append(ids, strconv.FormatUint(slot, 10))
		}
		return ids, true, nil
	}
	return nil, false, fmt.Errorf("%w: want block <id> or blocks <from> <to>", errUsage)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
