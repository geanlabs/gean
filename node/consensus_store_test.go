package node

import (
	"testing"

	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

func makeTestStore() *ConsensusStore {
	backend := storage.NewInMemoryBackend()
	s := NewConsensusStore(backend)
	s.SetConfig(&types.ChainConfig{GenesisTime: 1000})
	return s
}

func makeCheckpoint(rootByte byte, slot uint64) *types.Checkpoint {
	var root [32]byte
	root[0] = rootByte
	return &types.Checkpoint{Root: root, Slot: slot}
}

func makeHeader(slot, proposer uint64, parentRootByte byte) *types.BlockHeader {
	var parent [32]byte
	parent[0] = parentRootByte
	return &types.BlockHeader{
		Slot:          slot,
		ProposerIndex: proposer,
		ParentRoot:    parent,
	}
}

func TestMetadataRoundtrip(t *testing.T) {
	s := makeTestStore()

	s.SetTime(42)
	if s.Time() != 42 {
		t.Fatalf("time: expected 42, got %d", s.Time())
	}

	var root [32]byte
	root[0] = 0xab
	s.SetHead(root)
	if s.Head() != root {
		t.Fatal("head mismatch")
	}

	cp := makeCheckpoint(0xcd, 10)
	s.SetLatestJustified(cp)
	got := s.LatestJustified()
	if got.Root != cp.Root || got.Slot != cp.Slot {
		t.Fatal("justified mismatch")
	}

	cp2 := makeCheckpoint(0xef, 5)
	s.SetLatestFinalized(cp2)
	got2 := s.LatestFinalized()
	if got2.Root != cp2.Root || got2.Slot != cp2.Slot {
		t.Fatal("finalized mismatch")
	}
}

func TestBlockHeaderStorage(t *testing.T) {
	s := makeTestStore()
	var root [32]byte
	root[0] = 0x01
	h := makeHeader(5, 2, 0x00)

	s.InsertBlockHeader(root, h)
	got := s.GetBlockHeader(root)
	if got == nil {
		t.Fatal("header not found")
	}
	if got.Slot != 5 || got.ProposerIndex != 2 {
		t.Fatalf("header mismatch: slot=%d proposer=%d", got.Slot, got.ProposerIndex)
	}
}

func TestStateStorage(t *testing.T) {
	s := makeTestStore()
	var root [32]byte
	root[0] = 0x01

	state := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     10,
		LatestBlockHeader:        &types.BlockHeader{},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
	s.InsertState(root, state)
	if !s.HasState(root) {
		t.Fatal("state should exist")
	}
	got := s.GetState(root)
	if got == nil {
		t.Fatal("state not found")
	}
	if got.Slot != 10 {
		t.Fatalf("state slot mismatch: expected 10, got %d", got.Slot)
	}
}

func TestPayloadBufferPushAndExtract(t *testing.T) {
	pb := NewPayloadBuffer(100)
	var dr [32]byte
	dr[0] = 1
	data := &types.AttestationData{Slot: 5}
	participants := types.NewBitlistSSZ(3)
	types.BitlistSet(participants, 0)
	types.BitlistSet(participants, 2)
	proof := &types.AggregatedSignatureProof{Participants: participants}

	pb.Push(dr, data, proof)
	if pb.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", pb.Len())
	}

	atts := pb.ExtractLatestAttestations()
	if len(atts) != 2 {
		t.Fatalf("expected 2 validators, got %d", len(atts))
	}
	if atts[0].Slot != 5 || atts[2].Slot != 5 {
		t.Fatal("attestation data mismatch")
	}
}

func TestPayloadBufferFIFOEviction(t *testing.T) {
	pb := NewPayloadBuffer(2) // capacity 2 proofs

	for i := byte(0); i < 5; i++ {
		var dr [32]byte
		dr[0] = i
		data := &types.AttestationData{Slot: uint64(i)}
		bits := types.NewBitlistSSZ(1)
		types.BitlistSet(bits, 0)
		proof := &types.AggregatedSignatureProof{Participants: bits}
		pb.Push(dr, data, proof)
	}

	// Should have evicted old entries to stay under capacity.
	if pb.TotalProofs() > 2 {
		t.Fatalf("expected <= 2 proofs, got %d", pb.TotalProofs())
	}
}

func TestPromoteNewToKnown(t *testing.T) {
	s := makeTestStore()

	var dr [32]byte
	dr[0] = 1
	data := &types.AttestationData{Slot: 5}
	bits := types.NewBitlistSSZ(1)
	types.BitlistSet(bits, 0)
	proof := &types.AggregatedSignatureProof{Participants: bits}

	s.NewPayloads.Push(dr, data, proof)
	if s.NewPayloads.Len() != 1 {
		t.Fatal("expected 1 new payload")
	}
	if s.KnownPayloads.Len() != 0 {
		t.Fatal("known should be empty")
	}

	s.PromoteNewToKnown()

	if s.NewPayloads.Len() != 0 {
		t.Fatal("new should be empty after promote")
	}
	if s.KnownPayloads.Len() != 1 {
		t.Fatal("known should have 1 entry")
	}
}

func TestExtractLatestAllAttestations(t *testing.T) {
	s := makeTestStore()

	// Validator 0 in known at slot 5.
	var dr1 [32]byte
	dr1[0] = 1
	bits1 := types.NewBitlistSSZ(1)
	types.BitlistSet(bits1, 0)
	s.KnownPayloads.Push(dr1, &types.AttestationData{Slot: 5}, &types.AggregatedSignatureProof{Participants: bits1})

	// Validator 0 in new at slot 8 (newer).
	var dr2 [32]byte
	dr2[0] = 2
	bits2 := types.NewBitlistSSZ(1)
	types.BitlistSet(bits2, 0)
	s.NewPayloads.Push(dr2, &types.AttestationData{Slot: 8}, &types.AggregatedSignatureProof{Participants: bits2})

	all := s.ExtractLatestAllAttestations()
	if all[0].Slot != 8 {
		t.Fatalf("expected slot 8 (newer), got %d", all[0].Slot)
	}
}

func TestAttestationSignatureInsertAndDelete(t *testing.T) {
	gsm := make(AttestationSignatureMap)
	var dr [32]byte
	dr[0] = 1
	data := &types.AttestationData{Slot: 5}
	var sig [types.SignatureSize]byte

	gsm.Insert(dr, data, 0, sig)
	gsm.Insert(dr, data, 1, sig)

	if gsm.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", gsm.Len())
	}
	if len(gsm[dr].Signatures) != 2 {
		t.Fatal("expected 2 signatures")
	}

	gsm.Delete([]AttestationDeleteKey{{ValidatorID: 0, DataRoot: dr}})
	if len(gsm[dr].Signatures) != 1 {
		t.Fatal("expected 1 signature after delete")
	}
}

func TestAttestationSignaturePruneBelow(t *testing.T) {
	gsm := make(AttestationSignatureMap)
	var sig [types.SignatureSize]byte
	for i := uint64(0); i < 5; i++ {
		var dr [32]byte
		dr[0] = byte(i)
		gsm.Insert(dr, &types.AttestationData{Slot: i}, 0, sig)
	}

	pruned := gsm.PruneBelow(2) // remove slots 0, 1, 2
	if pruned != 3 {
		t.Fatalf("expected 3 pruned, got %d", pruned)
	}
	if gsm.Len() != 2 {
		t.Fatalf("expected 2 remaining, got %d", gsm.Len())
	}
}

func TestValidateAttestationDataAvailability(t *testing.T) {
	s := makeTestStore()
	data := &types.AttestationData{
		Slot:   5,
		Source: &types.Checkpoint{Root: [32]byte{1}, Slot: 3},
		Target: &types.Checkpoint{Root: [32]byte{2}, Slot: 4},
		Head:   &types.Checkpoint{Root: [32]byte{3}, Slot: 5},
	}

	// All blocks missing — should fail.
	err := ValidateAttestationData(s, data)
	if err == nil {
		t.Fatal("should fail with unknown blocks")
	}
	se, ok := err.(*StoreError)
	if !ok || se.Kind != ErrUnknownSourceBlock {
		t.Fatalf("expected UnknownSourceBlock, got %v", err)
	}
}

func TestValidateAttestationDataTopology(t *testing.T) {
	s := makeTestStore()
	s.SetTime(30) // slot ~6

	// Insert blocks for source, target, head.
	s.InsertBlockHeader([32]byte{1}, &types.BlockHeader{Slot: 3})
	s.InsertBlockHeader([32]byte{2}, &types.BlockHeader{Slot: 4})
	s.InsertBlockHeader([32]byte{3}, &types.BlockHeader{Slot: 5})

	// Valid attestation.
	data := &types.AttestationData{
		Slot:   5,
		Source: &types.Checkpoint{Root: [32]byte{1}, Slot: 3},
		Target: &types.Checkpoint{Root: [32]byte{2}, Slot: 4},
		Head:   &types.Checkpoint{Root: [32]byte{3}, Slot: 5},
	}
	if err := ValidateAttestationData(s, data); err != nil {
		t.Fatalf("should pass: %v", err)
	}

	// Source exceeds target.
	bad := *data
	bad.Source = &types.Checkpoint{Root: [32]byte{3}, Slot: 5}
	bad.Target = &types.Checkpoint{Root: [32]byte{1}, Slot: 3}
	err := ValidateAttestationData(s, &bad)
	if err == nil {
		t.Fatal("should fail: source exceeds target")
	}
}

func TestAggregationBitsFromValidatorIndices(t *testing.T) {
	bits := AggregationBitsFromIndices([]uint64{0, 3, 7})
	if !types.BitlistGet(bits, 0) || !types.BitlistGet(bits, 3) || !types.BitlistGet(bits, 7) {
		t.Fatal("expected bits 0, 3, 7 set")
	}
	if types.BitlistGet(bits, 1) || types.BitlistGet(bits, 5) {
		t.Fatal("bits 1, 5 should not be set")
	}
	if types.BitlistLen(bits) != 8 {
		t.Fatalf("expected length 8, got %d", types.BitlistLen(bits))
	}
}
