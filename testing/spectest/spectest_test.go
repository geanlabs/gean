package spectest

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeTree creates the case directories under a temporary root.
func writeTree(t *testing.T, paths ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Join(root, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

var tree = []string{
	"minimal/pq/operations/randao/pyspec_tests/valid",
	"minimal/pq/operations/randao/pyspec_tests/invalid",
	"minimal/pq/ssz_static/Checkpoint/ssz_random/case_0",
	"minimal/pq/sanity/blocks/pyspec_tests/one",
	"mainnet/pq/operations/randao/pyspec_tests/valid",
}

// Cases are the directories six levels down; a file at that depth and a directory above it are
// not cases.
func TestDiscover(t *testing.T) {
	root := writeTree(t, append(slices.Clone(tree), "minimal/pq/sanity/slots")...)
	stray := filepath.Join(root, "minimal/pq/operations/randao/pyspec_tests/README.md")
	if err := os.WriteFile(stray, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cases, err := discover(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range cases {
		got = append(got, c.Path())
	}
	slices.Sort(got)
	want := slices.Sorted(slices.Values(tree))
	if !slices.Equal(got, want) {
		t.Fatalf("cases = %v, want %v", got, want)
	}
}

func TestCheck(t *testing.T) {
	cases, err := discover(writeTree(t, append(slices.Clone(tree),
		"minimal/heze/sanity/blocks/pyspec_tests/one",
		"mainnet/heze/sanity/blocks/pyspec_tests/one",
	)...))
	if err != nil {
		t.Fatal(err)
	}
	r := registry{
		forks: []string{"pq"},
		handlers: map[string]func(*testing.T, Case){
			"operations/randao":  func(*testing.T, Case) {},
			"operations/deposit": func(*testing.T, Case) {},
			"ssz_static":         func(*testing.T, Case) {},
		},
		excluded: map[string]string{"sanity": "later", "light_client": "not in PQ"},
	}
	tests := []struct {
		name  string
		cases []Case
		want  []string
	}{
		{"tree", cases, []string{
			`exclusion light_client matches no case`,
			`fork directory "heze" is not one gean runs`,
			`handler operations/deposit matches no case`,
		}},
		{"empty", nil, []string{"no case found"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.check(tt.cases); !slices.Equal(got, tt.want) {
				t.Fatalf("check = %q, want %q", got, tt.want)
			}
		})
	}
}

// Every case runs as a subtest: a handled one passes, the others skip with their reason, and
// the summary counts each handler's outcomes.
func TestRun(t *testing.T) {
	root := writeTree(t, tree...)
	r := registry{
		forks: []string{"pq"},
		handlers: map[string]func(*testing.T, Case){
			"operations/randao": func(*testing.T, Case) {},
			"ssz_static":        func(t *testing.T, _ Case) { t.Skip("type not modelled") },
			"sanity":            func(t *testing.T, _ Case) { t.Error("sanity ran though excluded") },
		},
		excluded: map[string]string{"sanity/blocks": "not yet"},
	}
	var out strings.Builder
	t.Run("registry", func(t *testing.T) { r.run(t, root, &out) })
	for _, want := range []string{
		"spec tests: 3 passed, 0 failed, 2 skipped\n",
		"  minimal/pq/operations/randao: 2 passed, 0 failed, 0 skipped\n",
		"  minimal/pq/sanity/blocks: 0 passed, 0 failed, 1 skipped (excluded: 1)\n",
		"  minimal/pq/ssz_static/Checkpoint: 0 passed, 0 failed, 1 skipped " +
			"(skipped by handler: 1)\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("summary is missing %q:\n%s", want, out.String())
		}
	}
}

func TestSummary(t *testing.T) {
	s := &summary{counts: map[string]*counts{}}
	key := "minimal/pq/operations/randao"
	s.add(key, func(n *counts) { n.passed++ })
	s.add(key, func(n *counts) { n.failed += 2 })
	s.add(key, func(n *counts) { n.skips[skipNotImplemented]++ })
	want := "spec tests: 1 passed, 2 failed, 1 skipped\n" +
		"  minimal/pq/operations/randao: 1 passed, 2 failed, 1 skipped (not implemented: 1)\n"
	if got := s.String(); got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}

func TestPinnedCommit(t *testing.T) {
	dir := t.TempDir()
	pins := filepath.Join(dir, "pins.env")
	pinsFile := []byte("# comment\nCONSENSUS_SPECS_COMMIT=abc123\n")
	if err := os.WriteFile(pins, pinsFile, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := pinnedCommit(pins); err != nil || got != "abc123" {
		t.Fatalf("pinnedCommit = %q, %v; want abc123", got, err)
	}
	if _, err := pinnedCommit(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("pinnedCommit read a missing file")
	}
	if msg := missingFixtures("/nowhere", "abc123"); !strings.Contains(msg, "checkout abc123") {
		t.Fatalf("message does not check out the pin:\n%s", msg)
	}
}
