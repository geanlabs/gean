//go:build linux

package xmss

import (
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestProverJobsShareOneThread submits jobs from callers locked to distinct OS
// threads, the way gean's workers reach the prover from different threads, and
// requires every job to run on one thread that none of the callers own.
func TestProverJobsShareOneThread(t *testing.T) {
	const callers = 8
	var mu sync.Mutex
	callerTids := map[int]bool{}
	jobTids := map[int]bool{}

	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			mu.Lock()
			callerTids[syscall.Gettid()] = true
			mu.Unlock()
			for range 5 {
				onProverThread(func() {
					first := syscall.Gettid()
					// Give the scheduler a chance to move an unlocked goroutine.
					time.Sleep(time.Millisecond)
					mu.Lock()
					jobTids[first] = true
					jobTids[syscall.Gettid()] = true
					mu.Unlock()
				})
			}
		}()
	}
	wg.Wait()

	if len(callerTids) != callers {
		t.Fatalf("callers ran on %d threads, want %d distinct", len(callerTids), callers)
	}
	if len(jobTids) != 1 {
		t.Fatalf("prover jobs ran on %d OS threads, want 1", len(jobTids))
	}
	for tid := range jobTids {
		if callerTids[tid] {
			t.Fatalf("prover job ran on caller thread %d", tid)
		}
	}
}
