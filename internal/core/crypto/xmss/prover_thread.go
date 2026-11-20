package xmss

import (
	"runtime"
	"sync"
)

// Every prover call runs on one OS thread. With the arena on, leanVM gives each
// thread that proves its own region and keeps that thread's peak resident for the
// life of the process. A cgo call runs on whichever thread the scheduler picked
// for the calling goroutine, so proofs issued from different goroutines would each
// leave a peak behind on a different thread until the process runs out of memory.
// Proofs already run one at a time, so a single thread costs no parallelism.
var (
	proverJobs       = make(chan func())
	proverThreadOnce sync.Once
)

// onProverThread runs f on the prover thread and returns once it has finished.
func onProverThread(f func()) {
	proverThreadOnce.Do(func() { go runProverThread() })
	done := make(chan struct{})
	proverJobs <- func() {
		defer close(done)
		f()
	}
	<-done
}

func runProverThread() {
	runtime.LockOSThread()
	for job := range proverJobs {
		job()
	}
}
