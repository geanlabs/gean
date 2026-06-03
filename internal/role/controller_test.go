package role

import (
	"sync"
	"testing"
)

func TestController_InitialValue(t *testing.T) {
	if !New(true).Get() {
		t.Fatal("seeded=true but Get returned false")
	}
	if New(false).Get() {
		t.Fatal("seeded=false but Get returned true")
	}
}

func TestController_SetReturnsPrevious(t *testing.T) {
	c := New(false)
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

func TestController_Idempotent(t *testing.T) {
	c := New(true)
	if prev := c.Set(true); !prev {
		t.Fatalf("idempotent enable: prev=%v, want true", prev)
	}
	if !c.Get() {
		t.Fatal("idempotent enable drifted value")
	}

	c2 := New(false)
	if prev := c2.Set(false); prev {
		t.Fatalf("idempotent disable: prev=%v, want false", prev)
	}
	if c2.Get() {
		t.Fatal("idempotent disable drifted value")
	}
}

func TestController_NilReceiver(t *testing.T) {
	var c *Controller
	if c.Get() {
		t.Fatal("nil controller should read as false")
	}
	if prev := c.Set(true); prev {
		t.Fatal("nil controller Set should return false previous value")
	}
	c.notify(true)
}

func TestController_ChangeHook(t *testing.T) {
	var values []bool
	c := NewWithHook(false, func(v bool) {
		values = append(values, v)
	})

	c.Set(true)
	c.Set(true)
	c.Set(false)

	want := []bool{false, true, false}
	if len(values) != len(want) {
		t.Fatalf("hook calls=%v, want %v", values, want)
	}
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("hook call %d=%v, want %v", i, values[i], want[i])
		}
	}
}

func TestController_Concurrent(t *testing.T) {
	c := New(false)
	var wg sync.WaitGroup
	for i := range 64 {
		wg.Add(1)
		go func(v bool) {
			defer wg.Done()
			_ = c.Set(v)
			_ = c.Get()
		}(i%2 == 0)
	}
	wg.Wait()
}
