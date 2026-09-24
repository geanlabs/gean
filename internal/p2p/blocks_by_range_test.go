package p2p

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"

	"github.com/geanlabs/gean/internal/types"
)

type rangeRequestStream struct {
	network.Stream
	reader io.Reader
	writer io.Writer
}

func (s rangeRequestStream) Read(p []byte) (int, error)  { return s.reader.Read(p) }
func (s rangeRequestStream) Write(p []byte) (int, error) { return s.writer.Write(p) }
func (rangeRequestStream) SetReadDeadline(time.Time) error {
	return nil
}
func (rangeRequestStream) SetWriteDeadline(time.Time) error {
	return nil
}

func TestBlocksByRangeRequestSSZRoundtrip(t *testing.T) {
	req := &types.BlocksByRangeRequest{
		StartSlot: 100,
		Count:     32,
	}

	encoded, err := req.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(encoded) != 16 {
		t.Fatalf("expected 16 bytes, got %d", len(encoded))
	}

	decoded := &types.BlocksByRangeRequest{}
	if err := decoded.UnmarshalSSZ(encoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.StartSlot != 100 || decoded.Count != 32 {
		t.Fatalf("roundtrip mismatch: got start=%d count=%d, want start=100 count=32",
			decoded.StartSlot, decoded.Count)
	}
}

func TestBlocksByRangeRequestZeroCount(t *testing.T) {
	req := &types.BlocksByRangeRequest{StartSlot: 0, Count: 0}
	encoded, err := req.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(encoded) != 16 {
		t.Fatalf("expected 16 bytes, got %d", len(encoded))
	}
	decoded := &types.BlocksByRangeRequest{}
	if err := decoded.UnmarshalSSZ(encoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.StartSlot != 0 || decoded.Count != 0 {
		t.Fatalf("zero-count roundtrip mismatch: %+v", decoded)
	}
}

func TestBlocksByRangeRequestMaxValues(t *testing.T) {
	req := &types.BlocksByRangeRequest{
		StartSlot: 1<<63 + 7,
		Count:     types.MaxRequestBlocks,
	}
	encoded, err := req.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded := &types.BlocksByRangeRequest{}
	if err := decoded.UnmarshalSSZ(encoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.StartSlot != req.StartSlot || decoded.Count != req.Count {
		t.Fatalf("max-values roundtrip mismatch: got %+v, want %+v", decoded, req)
	}
}

func TestReadRangeRequestSizeLimit(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{name: "exact limit", size: MaxCompressedPayloadSize},
		{name: "over limit", size: MaxCompressedPayloadSize + 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readRangeRequest(bytes.NewReader(make([]byte, tt.size)))
			if (err != nil) != tt.wantErr {
				t.Fatalf("readRangeRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && len(got) != tt.size {
				t.Fatalf("readRangeRequest() returned %d bytes, want %d", len(got), tt.size)
			}
		})
	}
}

func TestBlocksByRangeRejectsOversizedRequest(t *testing.T) {
	var response bytes.Buffer
	stream := rangeRequestStream{
		reader: bytes.NewReader(make([]byte, MaxCompressedPayloadSize+1)),
		writer: &response,
	}
	lookupCalled := false

	handleBlocksByRangeRequest(stream, func() uint64 { return 0 }, func(uint64, uint64) ([]*types.SignedBlock, bool) {
		lookupCalled = true
		return nil, true
	})

	if lookupCalled {
		t.Fatal("block lookup called for oversized request")
	}
	code, message, err := DecodeResponse(&response)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if code != RespInvalidRequest {
		t.Fatalf("response code = %d, want %d", code, RespInvalidRequest)
	}
	if string(message) != "request too large" {
		t.Fatalf("response message = %q, want %q", message, "request too large")
	}
}
