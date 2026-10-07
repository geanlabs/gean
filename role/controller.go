package role

import (
	"sync/atomic"
)

type Controller struct {
	flag     atomic.Bool
	onChange func(bool)
}

func New(initial bool) *Controller {
	return NewWithHook(initial, nil)
}

func NewWithHook(initial bool, onChange func(bool)) *Controller {
	c := &Controller{onChange: onChange}
	c.flag.Store(initial)
	c.notify(initial)
	return c
}

func (c *Controller) Get() bool {
	if c == nil {
		return false
	}
	return c.flag.Load()
}

func (c *Controller) Set(v bool) bool {
	if c == nil {
		return false
	}
	prev := c.flag.Swap(v)
	if prev != v {
		c.notify(v)
	}
	return prev
}

func (c *Controller) notify(v bool) {
	if c != nil && c.onChange != nil {
		c.onChange(v)
	}
}
