package types

import (
	"bytes"
	"reflect"
	"testing"
)

type peerMessage interface {
	UnmarshalSSZ([]byte) error
	MarshalSSZ() ([]byte, error)
	HashTreeRoot() ([32]byte, error)
}

func seedAttestationData() *AttestationData {
	return &AttestationData{Slot: 3, Head: &Checkpoint{Slot: 3}, Target: &Checkpoint{Slot: 2}, Source: &Checkpoint{Slot: 1}}
}

// peerMessageSeeds holds one value of each type in this package that gean
// decodes from bytes a peer or a checkpoint-sync server sends. The fuzzer
// decodes into the type of the seed it picks.
var peerMessageSeeds = []peerMessage{
	&SignedBlock{
		Block: &Block{Slot: 1, Body: &BlockBody{Attestations: []*AggregatedAttestation{
			{AggregationBits: []byte{0b101}, Data: seedAttestationData()},
		}}},
		Proof: &MultiMessageAggregate{Proof: []byte{1, 2, 3}},
	},
	&SignedAttestation{ValidatorID: 7, Data: seedAttestationData()},
	&SignedAggregatedAttestation{Data: seedAttestationData(), Proof: &SingleMessageAggregate{Participants: []byte{0b11}, Proof: []byte{4}}},
	&BlocksByRootRequest{Roots: [][RootSize]byte{{1}, {2}}},
	&BlocksByRangeRequest{StartSlot: 5, Count: 10},
	testState(),
}

// FuzzSSZ decodes arbitrary bytes as each peer-facing type. Decoding must not
// panic, and bytes that decode must re-encode to themselves and have a root: a
// type that accepted a second encoding of one value would let a peer resend a
// message under a new gossip message ID.
func FuzzSSZ(f *testing.F) {
	for kind, seed := range peerMessageSeeds {
		encoded, err := seed.MarshalSSZ()
		if err != nil {
			f.Fatal(err)
		}
		f.Add(uint8(kind), encoded)
	}

	f.Fuzz(func(t *testing.T, kind uint8, data []byte) {
		seed := peerMessageSeeds[int(kind)%len(peerMessageSeeds)]
		v := reflect.New(reflect.TypeOf(seed).Elem()).Interface().(peerMessage)
		if v.UnmarshalSSZ(data) != nil {
			return
		}
		encoded, err := v.MarshalSSZ()
		if err != nil {
			t.Fatalf("%T decoded but does not encode: %v", v, err)
		}
		if !bytes.Equal(encoded, data) {
			t.Fatalf("%T decoded %x but encodes to %x", v, data, encoded)
		}
		if _, err := v.HashTreeRoot(); err != nil {
			t.Fatalf("%T decoded but has no hash tree root: %v", v, err)
		}
	})
}
