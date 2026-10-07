package tasks

import (
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

func TestWaitJoinsRunningWorkAndRefusesNewWork(t *testing.T) {
	var g Group
	var finished atomic.Bool
	release := make(chan struct{})
	if !g.Go(func() {
		<-release
		finished.Store(true)
	}) {
		t.Fatal("Go refused work on an open group")
	}

	go close(release)
	g.Wait()
	if !finished.Load() {
		t.Fatal("Wait returned before tracked work finished")
	}

	if g.Go(func() {}) {
		t.Fatal("Go accepted work after Wait")
	}
	if g.Do(func() {}) {
		t.Fatal("Do accepted work after Wait")
	}
}

func TestGoCriticalReportsErrorsAndPanics(t *testing.T) {
	var g Group
	failures := make(chan error, 2)
	g.GoCritical("returns", func() error { return errors.New("boom") }, func(err error) { failures <- err })
	g.GoCritical("panics", func() error { panic("bang") }, func(err error) { failures <- err })
	g.GoCritical("succeeds", func() error { return nil }, func(err error) { failures <- err })
	g.Wait()
	close(failures)

	var got []string
	for err := range failures {
		got = append(got, err.Error())
	}
	if len(got) != 2 {
		t.Fatalf("failures=%q, want one each from the error and the panic", got)
	}
	for _, want := range []string{"returns: boom", "panics panicked: bang"} {
		if !slices.ContainsFunc(got, func(s string) bool { return strings.HasPrefix(s, want) }) {
			t.Fatalf("failures=%q, missing %q", got, want)
		}
	}
}
