//go:build spectests

// Package lean runs gean against leanSpec's consensus test fixtures.
//
// TestSpec hands every case under consensus/ to the runner registered for its
// directory, or leaves the directory out with a listed reason. Each case
// passes, fails, or skips with one of the skip reasons below. The run fails
// when:
//   - a fixture directory has neither a runner nor an exclusion;
//   - a runner or exclusion has no fixtures;
//   - a case has a field its fixture type does not declare;
//   - a case skips without a listed reason.
//
// A summary prints after every run: outcomes per directory, skip reasons,
// exclusions, and the expectations gean does not evaluate yet in the cases
// that ran.
package lean

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/lean/specfixtures"
)

var runners = map[string]runner{
	"api_endpoint":                 newRunner(runAPIEndpoint, hashedKeys),
	"fork_choice":                  newRunner(runForkChoice, hashedKeys),
	"justifiability":               newRunner(runJustifiability, hashedKeys),
	"networking_codec":             newRunner(runNetworkingCodec, hashedKeys),
	"slot_clock":                   newRunner(runSlotClock, hashedKeys),
	"ssz":                          newRunner(runSSZ, keyBytes),
	"state_transition":             newRunner(runStateTransition, hashedKeys),
	"sync":                         newRunner(runSync, hashedKeys),
	"verify_signatures":            newRunner(runVerifySignatures, keyBytes),
	"verify_single_message_proofs": newRunner(runVerifySingleMessageProof, keyBytes),
}

var excludedDirectories = map[string]string{
	"gossipsub_handler":    "gean's gossipsub is go-libp2p-pubsub, whose handler state these vectors drive directly",
	"poseidon_permutation": "Poseidon is leanSpec's XMSS hash; gean runs leanVM's BLAKE2s XMSS (leanSpec#1210)",
}

// skipReason names why a case cannot run. A case may only skip for one of
// these reasons, so every skip shows up in the summary under its cause.
type skipReason string

const (
	// leanSpec still specifies Poseidon XMSS, with 52-byte public keys and
	// 2,536-byte signatures. gean, like every devnet client, runs leanVM's
	// BLAKE2s XMSS (32 and 1,208 bytes), so these fixtures cannot load until
	// leanSpec specifies BLAKE2s.
	skipPoseidonXMSS skipReason = "Poseidon XMSS fixture; gean runs leanVM's BLAKE2s XMSS " +
		"(https://github.com/leanEthereum/leanSpec/issues/1210)"
	skipNotImplemented   skipReason = "not implemented in gean"
	skipFollowsEthlambda skipReason = "gean follows ethlambda where it diverges from leanSpec"
)

// skip ends the case, recording why.
func skip(t *testing.T, reason skipReason, detail string) {
	t.Helper()
	caseRecords[t].skipReason = reason
	t.Skip(string(reason) + ": " + detail)
}

// caseRecord is what a running case reports to the summary. Cases run one at a
// time, so the records need no locking.
type caseRecord struct {
	skipReason skipReason
	unchecked  []string
}

var caseRecords = make(map[*testing.T]*caseRecord)

// runner runs one fixture case from its raw JSON.
type runner func(t *testing.T, raw json.RawMessage)

// How a suite uses validator public keys: hashed into states and blocks, which a
// Poseidon key's hash tree root stands in for, or as the key bytes themselves.
const (
	hashedKeys = false
	keyBytes   = true
)

// newRunner decodes a case into F, rejecting fields F does not declare: a
// field leanSpec renames or adds then fails the case instead of reading as
// zero. Cases carrying Poseidon XMSS material skip once decoded.
func newRunner[F any](run func(*testing.T, *F), needsKeyBytes bool) runner {
	return func(t *testing.T, raw json.RawMessage) {
		t.Helper()
		var fixture F
		if err := decodeStrict(raw, &fixture); err != nil {
			t.Fatalf("decode fixture: %v", err)
		}
		caseRecords[t].unchecked = uncheckedFields(reflect.ValueOf(fixture), "")
		skipPoseidonMaterial(t, raw, needsKeyBytes)
		run(t, &fixture)
	}
}

// uncheckedFields lists, by JSON path, every specfixtures.Unchecked
// expectation present in v.
func uncheckedFields(v reflect.Value, path string) []string {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			return uncheckedFields(v.Elem(), path)
		}
	case reflect.Slice:
		if v.Type() == reflect.TypeOf(specfixtures.Unchecked(nil)) {
			if v.Len() > 0 {
				return []string{path}
			}
			return nil
		}
		var fields []string
		for i := range v.Len() {
			fields = append(fields, uncheckedFields(v.Index(i), path)...)
		}
		return fields
	case reflect.Struct:
		var fields []string
		for i := range v.NumField() {
			field := v.Type().Field(i)
			if !field.IsExported() {
				continue
			}
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			fieldPath := path
			if !field.Anonymous {
				fieldPath = strings.TrimPrefix(path+"."+name, ".")
			}
			fields = append(fields, uncheckedFields(v.Field(i), fieldPath)...)
		}
		return fields
	}
	return nil
}

// mustParse fails the case on a malformed fixture value.
func mustParse[T any](t *testing.T, parse func(string) (T, error), s string) T {
	t.Helper()
	v, err := parse(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

func decodeStrict(raw []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}

// fixturesDirectory is where make leanSpec/fixtures unpacks the pinned
// fixtures. LEAN_SPEC_FIXTURES points the tests at another copy.
func fixturesDirectory() string {
	if directory := os.Getenv("LEAN_SPEC_FIXTURES"); directory != "" {
		return directory
	}
	return filepath.Join("..", "..", "..", "leanSpec", "fixtures")
}

func TestSpec(t *testing.T) {
	root := filepath.Join(fixturesDirectory(), "consensus")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		t.Skipf("no fixtures at %s; run make leanSpec/fixtures", root)
	}
	if err != nil {
		t.Fatal(err)
	}

	present := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		directory := entry.Name()
		present[directory] = true
		if _, excluded := excludedDirectories[directory]; excluded {
			continue
		}
		run, ok := runners[directory]
		if !ok {
			t.Errorf("fixtures in %s have no runner: add one, or exclude the directory with a reason", directory)
			continue
		}
		t.Run(directory, func(t *testing.T) { runDirectory(t, filepath.Join(root, directory), run) })
	}

	for directory := range runners {
		if !present[directory] {
			t.Errorf("runner for %s has no fixtures: has leanSpec renamed or dropped them?", directory)
		}
	}
	for directory := range excludedDirectories {
		if !present[directory] {
			t.Errorf("excluded directory %s has no fixtures: drop the exclusion", directory)
		}
	}
}

// runDirectory runs every case in every fixture file under directory.
func runDirectory(t *testing.T, directory string, run runner) {
	suite := filepath.Base(directory)
	cases := 0
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".json" {
			return err
		}
		fileCases, err := readCases(path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			return nil
		}
		relative, _ := filepath.Rel(directory, strings.TrimSuffix(path, ".json"))
		for _, key := range slices.Sorted(maps.Keys(fileCases)) {
			name := relative
			if len(fileCases) > 1 {
				name += "/" + caseLabel(key)
			}
			cases++
			raw := fileCases[key]
			t.Run(name, func(t *testing.T) {
				caseRecords[t] = &caseRecord{}
				t.Cleanup(func() { record(t, suite) })
				run(t, raw)
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if cases == 0 {
		t.Errorf("no fixture cases under %s", directory)
	}
}

// readCases reads a fixture file, a JSON object keyed by test ID.
func readCases(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases map[string]json.RawMessage
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

// caseLabel names a case within a file by its test parameters. A test ID looks
// like "tests/...py::test_name[fork_Lstar-two_of_six][fork_Lstar-fork_choice_test]",
// and the first bracket after the fork name holds the parameters.
func caseLabel(id string) string {
	params := id
	if open := strings.Index(id, "["); open >= 0 {
		if end := strings.Index(id[open:], "]"); end >= 0 {
			params = id[open+1 : open+end]
		}
	}
	if _, rest, found := strings.Cut(params, "-"); found {
		return rest
	}
	return params
}

// suites counts how the run's cases ended, by fixture directory.
var suites = make(map[string]*suiteOutcomes)

type suiteOutcomes struct {
	passed, failed, skipped int
	unchecked               map[string]int
}

var skipsByReason = make(map[skipReason]int)

// record counts a finished case. A case that skipped without a listed reason
// fails, since its skip would otherwise hide in the totals.
func record(t *testing.T, suite string) {
	c := caseRecords[t]
	delete(caseRecords, t)
	if t.Skipped() && !t.Failed() && c.skipReason == "" {
		t.Error("case skipped without a listed reason; skip through skip()")
	}

	s, ok := suites[suite]
	if !ok {
		s = &suiteOutcomes{unchecked: make(map[string]int)}
		suites[suite] = s
	}
	switch {
	case t.Failed():
		s.failed++
	case t.Skipped():
		s.skipped++
		skipsByReason[c.skipReason]++
		return
	default:
		s.passed++
	}
	for _, field := range c.unchecked {
		s.unchecked[field]++
	}
}

func writeSummary(w io.Writer) {
	if len(suites) == 0 {
		return
	}
	var passed, failed, skipped int
	fmt.Fprintf(w, "\nleanSpec fixtures (%s)\n", fixturesDirectory())
	fmt.Fprintf(w, "  %-30s %6s %6s %6s\n", "suite", "pass", "fail", "skip")
	for _, name := range slices.Sorted(maps.Keys(suites)) {
		s := suites[name]
		passed, failed, skipped = passed+s.passed, failed+s.failed, skipped+s.skipped
		fmt.Fprintf(w, "  %-30s %6d %6d %6d\n", name, s.passed, s.failed, s.skipped)
	}
	fmt.Fprintf(w, "  %-30s %6d %6d %6d\n", "total", passed, failed, skipped)
	for _, reason := range slices.Sorted(maps.Keys(skipsByReason)) {
		fmt.Fprintf(w, "  skipped %4d: %s\n", skipsByReason[reason], reason)
	}
	for _, directory := range slices.Sorted(maps.Keys(excludedDirectories)) {
		fmt.Fprintf(w, "  excluded: %s, %s\n", directory, excludedDirectories[directory])
	}
	for _, name := range slices.Sorted(maps.Keys(suites)) {
		unchecked := suites[name].unchecked
		for _, field := range slices.Sorted(maps.Keys(unchecked)) {
			fmt.Fprintf(w, "  unchecked: %s %s, in %d cases that ran\n", name, field, unchecked[field])
		}
	}
}

func TestMain(m *testing.M) {
	logger.SetOutput(io.Discard)
	code := m.Run()
	writeSummary(os.Stdout)
	os.Exit(code)
}
