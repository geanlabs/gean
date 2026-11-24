//go:build spectests

package spectest

import (
	"os"
	"path/filepath"
	"testing"
)

// supported is what gean runs. A case with no handler here skips as not implemented.
var supported = registry{forks: []string{"pq"}}

func TestSpec(t *testing.T) {
	root := os.Getenv("GEAN_SPEC_TESTS_DIR")
	if root == "" {
		root = filepath.Join("..", "..", "spectests")
	}
	if _, err := os.Stat(root); err != nil {
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
		commit, err := pinnedCommit(filepath.Join("..", "..", "pins.env"))
		if err != nil {
			t.Fatal(err)
		}
		msg := missingFixtures(root, commit)
		if os.Getenv("GEAN_REQUIRE_SPEC_TESTS") == "1" {
			t.Fatal(msg)
		}
		t.Skip(msg)
	}
	supported.run(t, root, os.Stdout)
}
