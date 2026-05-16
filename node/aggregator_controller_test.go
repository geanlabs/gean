package node

import (
	"sync"
	"testing"
)

func TestAggregatorController_InitialValue(t *testing.T) {
	if !NewAggregatorController(true).Get() {
		t.Fatal("seeded=true but Get returned false")
	}
	if NewAggregatorController(false).Get() {
		t.Fatal("seeded=false but Get returned true")
	}
}

func TestAggregatorController_SetReturnsPrevious(t *testing.T) {
	c := NewAggregatorController(false)
	if prev := c.Set(true); prev {
		t.Fatalf("first flip: prev=%v, want false", prev)
	}
	if !c.Get() {
		t.Fatal("after Set(true), Get returned false")
	}
	if prev := c.Set(false); !prev {
		t.Fatalf("second flip: prev=%v, want true", prev)
	}
	if c.Get() {
		t.Fatal("after Set(false), Get returned true")
	}
}

func TestAggregatorController_Idempotent(t *testing.T) {
	c := NewAggregatorController(true)
	if prev := c.Set(true); !prev {
		t.Fatalf("idempotent enable: prev=%v, want true", prev)
	}
	if !c.Get() {
		t.Fatal("idempotent enable drifted value")
	}

	c2 := NewAggregatorController(false)
	if prev := c2.Set(false); prev {
		t.Fatalf("idempotent disable: prev=%v, want false", prev)
	}
	if c2.Get() {
		t.Fatal("idempotent disable drifted value")
	}
}

// TestAggregatorController_Concurrent exercises the atomic read/write path
// under -race. Final state isn't deterministic; success is surviving the
// race detector without a data race or panic.
func TestAggregatorController_Concurrent(t *testing.T) {
	c := NewAggregatorController(false)
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(v bool) {
			defer wg.Done()
			_ = c.Set(v)
			_ = c.Get()
		}(i%2 == 0)
	}
	wg.Wait()
}
