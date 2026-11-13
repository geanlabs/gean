package validator

import "testing"

func TestProposalReservation(t *testing.T) {
	var r ProposalReservation
	slot1 := ProposalDuty{Slot: 1, ValidatorID: 7}
	slot2 := ProposalDuty{Slot: 2, ValidatorID: 7}

	if !r.Available(1) {
		t.Fatal("slot 1 unavailable before any reservation")
	}
	r.Reserve(slot1)
	if r.Available(1) {
		t.Fatal("reserved slot 1 still available")
	}
	if r.Available(0) {
		t.Fatal("slot older than the reservation available")
	}
	if !r.Available(2) {
		t.Fatal("later slot blocked by slot 1's reservation")
	}

	r.Reserve(slot2)
	if r.Release(slot1) {
		t.Fatal("stale slot 1 result released slot 2's reservation")
	}
	if r.Available(2) {
		t.Fatal("slot 2 available after a stale release")
	}
	if !r.Release(slot2) || !r.Available(2) {
		t.Fatal("pre-sign failure did not release slot 2")
	}
}
