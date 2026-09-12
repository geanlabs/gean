package xmss

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestConfigureProverThreads(t *testing.T) {
	// Each subprocess starts with an unresolved native pool.
	raw := os.Getenv("GEAN_TEST_PROVER_THREADS")
	if raw == "" {
		for _, count := range []string{"0", "1"} {
			t.Run(count, func(t *testing.T) {
				cmd := exec.Command(os.Args[0], "-test.run=^TestConfigureProverThreads$", "-test.v")
				cmd.Env = append(os.Environ(), "GEAN_TEST_PROVER_THREADS="+count)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v\n%s", err, out)
				}
			})
		}
		return
	}
	requested, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ConfigureProverThreads(-1); err == nil {
		t.Fatal("accepted negative count")
	}
	if _, err := ConfigureProverThreads(int(^uint(0) >> 1)); err == nil {
		t.Fatal("accepted excessive count")
	}
	actual, err := ConfigureProverThreads(requested)
	if err != nil {
		t.Fatal(err)
	}
	if actual < 1 || (requested != 0 && actual != requested) {
		t.Fatalf("resolved %d for %d", actual, requested)
	}
	if err := EnsureVerifierReady(); err != nil {
		t.Fatal(err)
	}
	if err := EnsureProverReady(); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "linux" {
		paths, err := filepath.Glob("/proc/self/task/*/comm")
		if err != nil {
			t.Fatal(err)
		}
		workers := 0
		for _, path := range paths {
			name, err := os.ReadFile(path)
			if err != nil {
				continue
			} // An unrelated Go thread may exit during enumeration.
			if strings.HasPrefix(string(name), "parallel-worker") {
				workers++
			}
		}
		if workers != actual-1 {
			t.Fatalf("native workers = %d, want %d", workers, actual-1)
		}
		t.Logf("native pool: %d background workers + caller", workers)
	}
	if got, err := ConfigureProverThreads(actual); err != nil || got != actual {
		t.Fatalf("same count: %d, %v", got, err)
	}
	different := 1
	if actual == 1 {
		different = 2
	}
	if _, err := ConfigureProverThreads(different); err == nil {
		t.Fatal("changed initialized pool")
	}
	t.Run("aggregate", TestAggregateSignaturesRoundtrip)
	t.Run("merge_and_split", TestType2Roundtrip)
}
