package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/geanlabs/gean/internal/zkstf"
	"github.com/geanlabs/gean/internal/zkstf/zkvectors"
)

// manifest lists framed inputs with their expected outcome, as computed by the
// native transition when the vectors were written.
type manifest struct {
	Cases []manifestCase `json:"cases"`
}

type manifestCase struct {
	Name         string `json:"name"`
	Input        string `json:"input"` // relative to the manifest
	WantErr      bool   `json:"wantErr"`
	PublicValues string `json:"publicValues,omitempty"`
}

func runVectors(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("vectors", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "zk/out/vectors", "output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cases, err := zkvectors.Suite()
	if err != nil {
		return err
	}
	var m manifest
	for _, c := range cases {
		input, err := zkstf.NewInput(c.Pre, c.Block)
		if err != nil {
			return fmt.Errorf("%s: %w", c.Name, err)
		}
		mc := manifestCase{Name: c.Name, Input: c.Name + ".bin", WantErr: c.WantErr}
		if !c.WantErr {
			pv, err := zkstf.Apply(input)
			if err != nil {
				return fmt.Errorf("%s: %w", c.Name, err)
			}
			b := pv.Bytes()
			mc.PublicValues = "0x" + hex.EncodeToString(b[:])
		}
		path := filepath.Join(*out, mc.Input)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, input, 0o644); err != nil {
			return err
		}
		m.Cases = append(m.Cases, mc)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*out, "manifest.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %d cases to %s\n", len(m.Cases), *out)
	return nil
}

func loadManifest(path string) (*manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &m, nil
}
