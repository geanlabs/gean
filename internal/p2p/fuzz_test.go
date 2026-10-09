package p2p

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

// FuzzDecodeGossipBlock checks that a block gossip accepts re-encodes to the
// exact bytes received, so one block cannot travel under two message IDs.
func FuzzDecodeGossipBlock(f *testing.F) {
	data := &types.AttestationData{
		Slot:   3,
		Head:   &types.Checkpoint{Root: [types.RootSize]byte{1}, Slot: 3},
		Target: &types.Checkpoint{Root: [types.RootSize]byte{2}, Slot: 2},
		Source: &types.Checkpoint{Root: [types.RootSize]byte{3}, Slot: 1},
	}
	seeds := []*types.SignedBlock{
		{Block: &types.Block{Body: &types.BlockBody{}}, Proof: &types.MultiMessageAggregate{}},
		{
			Block: &types.Block{Slot: 4, Body: &types.BlockBody{Attestations: []*types.AggregatedAttestation{
				{AggregationBits: []byte{0x0b}, Data: data},
			}}},
			Proof: &types.MultiMessageAggregate{Proof: []byte{1, 2, 3}},
		},
	}
	for _, seed := range seeds {
		enc, err := seed.MarshalSSZ()
		if err != nil {
			f.Fatalf("marshal seed: %v", err)
		}
		f.Add(enc)
	}
	f.Add(paddedEmptyListBlock())

	h := &Host{}
	f.Fuzz(func(t *testing.T, in []byte) {
		decoded, err := h.decodeGossip(BlockTopic(), SnappyRawEncode(in))
		if err != nil {
			return
		}
		out, err := decoded.(*types.SignedBlock).MarshalSSZ()
		if err != nil {
			t.Fatalf("accepted block does not re-encode: %v", err)
		}
		if !bytes.Equal(out, in) {
			t.Fatalf("accepted block re-encodes to %x, received %x", out, in)
		}
	})
}

// FuzzDecodeReqRespPayload checks that a decoded payload survives a round trip.
func FuzzDecodeReqRespPayload(f *testing.F) {
	f.Add(EncodeReqRespPayload(nil))
	f.Add(EncodeReqRespPayload([]byte("blocks_by_range")))
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0x0f})

	f.Fuzz(func(t *testing.T, in []byte) {
		decoded, err := DecodeReqRespPayload(in)
		if err != nil {
			return
		}
		again, err := DecodeReqRespPayload(EncodeReqRespPayload(decoded))
		if err != nil || !bytes.Equal(again, decoded) {
			t.Fatalf("re-encoded payload decodes to %x (err %v), want %x", again, err, decoded)
		}
	})
}

// FuzzDecodeResponse checks that a decoded response survives a round trip.
func FuzzDecodeResponse(f *testing.F) {
	f.Add(EncodeResponse(RespSuccess, []byte("block")))
	f.Add(EncodeResponse(RespSuccess, nil))
	f.Add(EncodeResponse(RespResourceUnavailable, []byte("unavailable")))

	f.Fuzz(func(t *testing.T, in []byte) {
		code, decoded, err := DecodeResponse(bytes.NewReader(in))
		if err != nil {
			return
		}
		gotCode, again, err := DecodeResponse(bytes.NewReader(EncodeResponse(code, decoded)))
		if err != nil || gotCode != code || !bytes.Equal(again, decoded) {
			t.Fatalf("re-encoded response decodes to code=%d %x (err %v), want code=%d %x", gotCode, again, err, code, decoded)
		}
	})
}

// paddedEmptyListBlock encodes an empty SignedBlock whose empty attestation
// list is written as a 4-byte zero offset instead of no bytes.
func paddedEmptyListBlock() []byte {
	block := make([]byte, 84)
	binary.LittleEndian.PutUint32(block[80:], 84)
	block = append(block, 4, 0, 0, 0, 0, 0, 0, 0)

	signed := make([]byte, 8)
	binary.LittleEndian.PutUint32(signed[0:], 8)
	binary.LittleEndian.PutUint32(signed[4:], uint32(8+len(block)))
	signed = append(signed, block...)
	return append(signed, 4, 0, 0, 0)
}
