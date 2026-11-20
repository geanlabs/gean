package types

import "bytes"

// Copy returns a deep copy of d, or nil for a nil d.
func (d *AttestationData) Copy() *AttestationData {
	if d == nil {
		return nil
	}
	return &AttestationData{
		Slot:   d.Slot,
		Head:   d.Head.Copy(),
		Target: d.Target.Copy(),
		Source: d.Source.Copy(),
	}
}

// Copy returns a copy of c, or nil for a nil c.
func (c *Checkpoint) Copy() *Checkpoint {
	if c == nil {
		return nil
	}
	out := *c
	return &out
}

// Copy returns a deep copy of a, or nil for a nil a.
func (a *SingleMessageAggregate) Copy() *SingleMessageAggregate {
	if a == nil {
		return nil
	}
	return &SingleMessageAggregate{
		Participants: bytes.Clone(a.Participants),
		Proof:        bytes.Clone(a.Proof),
	}
}
