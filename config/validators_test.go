package config

import (
	"strings"
	"testing"
)

func TestValidateHappyPath(t *testing.T) {
	reg := &ValidatorRegistry{
		Assignments: []ValidatorAssignment{
			{NodeName: "node-a", Validators: []uint64{0, 1}},
			{NodeName: "node-b", Validators: []uint64{2, 3}},
		},
	}
	if err := reg.Validate(5); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateOutOfRange(t *testing.T) {
	reg := &ValidatorRegistry{
		Assignments: []ValidatorAssignment{
			{NodeName: "node-a", Validators: []uint64{0, 5}},
		},
	}
	err := reg.Validate(5)
	if err == nil {
		t.Fatal("expected error for out-of-range validator index")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected 'out of range' in error, got: %v", err)
	}
}

func TestValidateOverlap(t *testing.T) {
	reg := &ValidatorRegistry{
		Assignments: []ValidatorAssignment{
			{NodeName: "node-a", Validators: []uint64{0, 1}},
			{NodeName: "node-b", Validators: []uint64{1, 2}},
		},
	}
	err := reg.Validate(5)
	if err == nil {
		t.Fatal("expected error for overlapping validator assignments")
	}
	if !strings.Contains(err.Error(), "assigned to both") {
		t.Fatalf("expected 'assigned to both' in error, got: %v", err)
	}
}

func TestValidateEmptyAssignments(t *testing.T) {
	reg := &ValidatorRegistry{
		Assignments: []ValidatorAssignment{},
	}
	if err := reg.Validate(5); err != nil {
		t.Fatalf("expected nil for empty assignments, got %v", err)
	}
}

func TestValidateZeroGenesisValidators(t *testing.T) {
	reg := &ValidatorRegistry{
		Assignments: []ValidatorAssignment{
			{NodeName: "node-a", Validators: []uint64{0}},
		},
	}
	err := reg.Validate(0)
	if err == nil {
		t.Fatal("expected error when numGenesisValidators is 0")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected 'out of range' in error, got: %v", err)
	}
}

func TestGetValidatorIndicesKnownNode(t *testing.T) {
	reg := &ValidatorRegistry{
		Assignments: []ValidatorAssignment{
			{NodeName: "node-a", Validators: []uint64{0, 1}},
			{NodeName: "node-b", Validators: []uint64{2, 3}},
		},
	}
	got := reg.GetValidatorIndices("node-b")
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("expected [2, 3], got %v", got)
	}
}

func TestGetValidatorIndicesUnknownNode(t *testing.T) {
	reg := &ValidatorRegistry{
		Assignments: []ValidatorAssignment{
			{NodeName: "node-a", Validators: []uint64{0, 1}},
		},
	}
	got := reg.GetValidatorIndices("node-z")
	if got != nil {
		t.Fatalf("expected nil for unknown node, got %v", got)
	}
}
