// Package spectest runs gean against the consensus-specs reference tests.
//
// The generator writes each case to <preset>/<fork>/<runner>/<handler>/<suite>/<case>/, and every
// case becomes a subtest of that name. A registry maps a runner, or a runner and handler, to the
// code that runs its cases; a case nothing runs skips with its reason, and the summary printed
// after the run counts every handler's outcomes. The run fails when the tree holds no case, a
// fork gean does not run, or nothing for a registered handler or exclusion: a suite that passes
// because it found nothing to run is worse than one that fails.
package spectest

import (
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// Case is one reference test case: the directory holding its files, and where it sits in the
// tree.
type Case struct {
	Preset, Fork, Runner, Handler, Suite, Name string
	Dir                                        string
}

// Path is the case's place in the tree, which is also its subtest name.
func (c Case) Path() string {
	return strings.Join([]string{c.Preset, c.Fork, c.Runner, c.Handler, c.Suite, c.Name}, "/")
}

// discover lists every case under root, which holds one directory per preset. A case is a
// directory six levels down; files on the way there are not cases.
func discover(root string) ([]Case, error) {
	var cases []Case
	err := fs.WalkDir(os.DirFS(root), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || p == "." {
			return err
		}
		parts := strings.Split(p, "/")
		if len(parts) < 6 {
			return nil
		}
		cases = append(cases, Case{
			Preset: parts[0], Fork: parts[1], Runner: parts[2], Handler: parts[3],
			Suite: parts[4], Name: parts[5], Dir: filepath.Join(root, filepath.FromSlash(p)),
		})
		return fs.SkipDir
	})
	return cases, err
}

// Why a case skips. The summary counts skips by reason.
const (
	skipNotImplemented = "not implemented"
	skipExcluded       = "excluded"
	skipByHandler      = "skipped by handler"
)

// registry is what gean runs: the forks it implements, and for a runner/handler, or a runner as
// a whole, the code that runs its cases or why gean leaves them out.
type registry struct {
	forks    []string
	handlers map[string]func(*testing.T, Case)
	excluded map[string]string
}

// lookup returns the code that runs c, or the reason and detail it skips with. A runner/handler
// entry wins over the runner's.
func (r registry) lookup(c Case) (run func(*testing.T, Case), reason, detail string) {
	for _, key := range []string{c.Runner + "/" + c.Handler, c.Runner} {
		if why, ok := r.excluded[key]; ok {
			return nil, skipExcluded, why
		}
		if run := r.handlers[key]; run != nil {
			return run, "", ""
		}
	}
	return nil, skipNotImplemented, c.Runner + "/" + c.Handler
}

// check reports what the tree and the registry do not account for.
func (r registry) check(cases []Case) []string {
	if len(cases) == 0 {
		return []string{"no case found"}
	}
	keys := map[string]bool{}
	unknownForks := map[string]bool{}
	for _, c := range cases {
		keys[c.Runner] = true
		keys[c.Runner+"/"+c.Handler] = true
		if !slices.Contains(r.forks, c.Fork) {
			unknownForks[c.Fork] = true
		}
	}
	var problems []string
	for fork := range unknownForks {
		problems = append(problems, fmt.Sprintf("fork directory %q is not one gean runs", fork))
	}
	for key := range r.handlers {
		if !keys[key] {
			problems = append(problems, fmt.Sprintf("handler %s matches no case", key))
		}
	}
	for key := range r.excluded {
		if !keys[key] {
			problems = append(problems, fmt.Sprintf("exclusion %s matches no case", key))
		}
	}
	slices.Sort(problems)
	return problems
}

// run runs every case under root as its own parallel subtest of t, and writes the summary to w
// once they have all finished.
func (r registry) run(t *testing.T, root string, w io.Writer) {
	t.Helper()
	cases, err := discover(root)
	if err != nil {
		t.Fatalf("list cases under %s: %v", root, err)
	}
	for _, p := range r.check(cases) {
		t.Error(p)
	}
	sum := &summary{counts: map[string]*counts{}}
	t.Cleanup(func() { fmt.Fprint(w, sum) })
	for _, c := range cases {
		t.Run(c.Path(), func(t *testing.T) {
			t.Parallel()
			r.runCase(t, c, sum)
		})
	}
}

// runCase runs c, or skips it with its reason, and counts the outcome.
func (r registry) runCase(t *testing.T, c Case, sum *summary) {
	key := strings.Join([]string{c.Preset, c.Fork, c.Runner, c.Handler}, "/")
	run, reason, detail := r.lookup(c)
	if run == nil {
		sum.add(key, func(n *counts) { n.skips[reason]++ })
		t.Skipf("%s: %s", reason, detail)
		return
	}
	// A handler that skips or fails ends its goroutine, so its outcome is counted on the way out.
	defer func() {
		sum.add(key, func(n *counts) {
			switch {
			case t.Skipped():
				n.skips[skipByHandler]++
			case t.Failed():
				n.failed++
			default:
				n.passed++
			}
		})
	}()
	run(t, c)
}

// summary counts outcomes per preset/fork/runner/handler. Cases run in parallel, so it is locked.
type summary struct {
	mu     sync.Mutex
	counts map[string]*counts
}

type counts struct {
	passed, failed int
	skips          map[string]int
}

// add updates key's counts.
func (s *summary) add(key string, update func(*counts)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.counts[key]
	if n == nil {
		n = &counts{skips: map[string]int{}}
		s.counts[key] = n
	}
	update(n)
}

func (s *summary) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	var passed, failed, skippedTotal int
	for _, key := range slices.Sorted(maps.Keys(s.counts)) {
		n := s.counts[key]
		var reasons []string
		skipped := 0
		for _, reason := range slices.Sorted(maps.Keys(n.skips)) {
			reasons = append(reasons, fmt.Sprintf("%s: %d", reason, n.skips[reason]))
			skipped += n.skips[reason]
		}
		fmt.Fprintf(&b, "  %s: %d passed, %d failed, %d skipped", key, n.passed, n.failed, skipped)
		if len(reasons) > 0 {
			fmt.Fprintf(&b, " (%s)", strings.Join(reasons, ", "))
		}
		b.WriteString("\n")
		passed, failed, skippedTotal = passed+n.passed, failed+n.failed, skippedTotal+skipped
	}
	return fmt.Sprintf("spec tests: %d passed, %d failed, %d skipped\n%s",
		passed, failed, skippedTotal, b.String())
}

// pinnedCommit reads CONSENSUS_SPECS_COMMIT from the pins.env at path.
func pinnedCommit(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(string(data)) {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "CONSENSUS_SPECS_COMMIT="); ok {
			return v, nil
		}
	}
	return "", fmt.Errorf("no CONSENSUS_SPECS_COMMIT in %s", path)
}

// missingFixtures says how to generate the reference tests expected at root, from consensus-specs
// at commit.
func missingFixtures(root, commit string) string {
	return fmt.Sprintf(`no reference tests at %s.
Generate them from consensus-specs at the commit pins.env pins:
    git -C <consensus-specs> fetch origin %[2]s && git -C <consensus-specs> checkout %[2]s
    make -C <consensus-specs> test reftests=true fork=pq
then set GEAN_SPEC_TESTS_DIR=<consensus-specs>/reftests, or move that reftests directory to
spectests/ at the repository root`, root, commit)
}
