//go:build spectests

package spectest

import (
	"os"
	"path/filepath"
	"testing"
)

// supported is what gean runs. A case with no handler here skips as not implemented.
var supported = registry{forks: []string{"pq"}}

// TestSpec runs the reference tests in spectests/, which make spec-pq generates and stamps with the
// pinned commit, or in GEAN_SPEC_TESTS_DIR, which is taken as it is.
func TestSpec(t *testing.T) {
	root := os.Getenv("GEAN_SPEC_TESTS_DIR")
	stamp := ""
	if root == "" {
		commit, err := pinnedCommit(filepath.Join("..", "..", "pins.env"))
		if err != nil {
			t.Fatal(err)
		}
		root, stamp = filepath.Join("..", "..", "spectests"), commit
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if problem := fixturesProblem(root, stamp); problem != "" {
		if os.Getenv("GEAN_REQUIRE_SPEC_TESTS") == "1" {
			t.Fatal(problem)
		}
		t.Skip(problem)
	}
	supported.run(t, root, os.Stdout)
}
