package reqresp

import (
	"bytes"
	"encoding/binary"
	"io"
	"math/rand/v2"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/libp2p/go-libp2p/core/network"
)

// stubStream is a minimal network.Stream for exercising WriteResponse: it wires
// Write and SetWriteDeadline (the only methods WriteResponse touches) and leaves the
// rest to the embedded nil interface, which panics if unexpectedly called.
type stubStream struct {
	network.Stream
	w        io.Writer
	writeErr error
}

func (s stubStream) Write(p []byte) (int, error) {
	if s.writeErr != nil {
		return 0, s.writeErr
	}
	return s.w.Write(p)
}

func (stubStream) SetWriteDeadline(time.Time) error { return nil }

func TestVarintRoundtrip(t *testing.T) {
	tests := []uint32{0, 1, 127, 128, 255, 256, 16383, 16384, 1<<20 - 1}
	for _, v := range tests {
		r := bytes.NewReader(binary.AppendUvarint(nil, uint64(v)))
		decoded, err := decodeVarintFrom(r)
		if err != nil {
			t.Fatalf("varint %d: %v", v, err)
		}
		if decoded != v {
			t.Fatalf("varint %d: got %d", v, decoded)
		}
		if r.Len() != 0 {
			t.Fatalf("varint %d: %d trailing bytes", v, r.Len())
		}
	}
}

func TestReqRespPayloadRoundtrip(t *testing.T) {
	data := []byte("state transition function data")
	encoded := EncodePayload(data)
	decoded, err := DecodePayload(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(data, decoded) {
		t.Fatal("payload roundtrip mismatch")
	}
}

func TestResponseEncoding(t *testing.T) {
	data := []byte("response payload")
	encoded := EncodeResponse(RespSuccess, data)
	if encoded[0] != RespSuccess {
		t.Fatalf("expected code 0x00, got 0x%02x", encoded[0])
	}

	reader := bytes.NewReader(encoded)
	code, decoded, err := DecodeResponse(reader, MaxPayloadSize)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if code != RespSuccess {
		t.Fatalf("code: expected 0, got %d", code)
	}
	if !bytes.Equal(data, decoded) {
		t.Fatal("response roundtrip mismatch")
	}
}

func TestWriteResponseReportsWriteFailure(t *testing.T) {
	if WriteResponse(stubStream{writeErr: os.ErrClosed}, "test", RespSuccess, []byte("payload")) {
		t.Fatal("expected WriteResponse to report failure")
	}
}

func TestWriteResponseWritesEncodedResponse(t *testing.T) {
	var buf bytes.Buffer
	if !WriteResponse(stubStream{w: &buf}, "test", RespSuccess, []byte("payload")) {
		t.Fatal("expected WriteResponse success")
	}
	code, payload, err := DecodeResponse(&buf, MaxPayloadSize)
	if err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if code != RespSuccess || !bytes.Equal(payload, []byte("payload")) {
		t.Fatalf("code=%d payload=%q, want success/payload", code, payload)
	}
}

func TestErrorMessageTruncatedOnEncode(t *testing.T) {
	oversized := bytes.Repeat([]byte("A"), MaxErrorMessageSize+100)
	encoded := EncodeResponse(RespInvalidRequest, oversized)
	code, decoded, err := DecodeResponse(bytes.NewReader(encoded), MaxPayloadSize)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if code != RespInvalidRequest {
		t.Fatalf("code: expected %d, got %d", RespInvalidRequest, code)
	}
	if len(decoded) != MaxErrorMessageSize {
		t.Fatalf("decoded length: expected %d, got %d", MaxErrorMessageSize, len(decoded))
	}
}

func TestErrorMessageAtBoundaryRoundtrips(t *testing.T) {
	data := bytes.Repeat([]byte("B"), MaxErrorMessageSize)
	encoded := EncodeResponse(RespServerError, data)
	code, decoded, err := DecodeResponse(bytes.NewReader(encoded), MaxPayloadSize)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if code != RespServerError {
		t.Fatalf("code: expected %d, got %d", RespServerError, code)
	}
	if !bytes.Equal(data, decoded) {
		t.Fatal("boundary-size error message roundtrip mismatch")
	}
}

func TestErrorMessageOversizedRejectedOnDecode(t *testing.T) {
	oversized := bytes.Repeat([]byte("C"), MaxErrorMessageSize+1)
	encoded := EncodeResponse(RespSuccess, oversized)
	encoded[0] = RespServerError
	_, _, err := DecodeResponse(bytes.NewReader(encoded), MaxPayloadSize)
	if err == nil {
		t.Fatal("expected error for oversize error-body decode, got nil")
	}
}

func TestReqRespPayloadRejectsBlockFormatSnappy(t *testing.T) {
	raw := []byte("hello world")
	blockFormat := snappy.Encode(nil, raw)
	chunk := append(binary.AppendUvarint(nil, uint64(len(raw))), blockFormat...)
	_, err := DecodePayload(chunk)
	if err == nil {
		t.Fatal("expected error decoding block-format snappy as framed, got nil")
	}
}

func TestReqRespPayloadRejectsDeclaredLengthMismatch(t *testing.T) {
	encoded := EncodePayload([]byte("x"))
	encoded[0] = 0
	_, err := DecodePayload(encoded)
	if err == nil {
		t.Fatal("expected length mismatch error")
	}
}

func TestReqRespPayloadRejectsDecodedBodyPastDeclaredLength(t *testing.T) {
	var framed bytes.Buffer
	w := snappy.NewBufferedWriter(&framed)
	if _, err := w.Write(bytes.Repeat([]byte("x"), 1024)); err != nil {
		t.Fatalf("write snappy payload: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close snappy writer: %v", err)
	}

	encoded := append(binary.AppendUvarint(nil, 1), framed.Bytes()...)
	_, err := DecodePayload(encoded)
	if err == nil {
		t.Fatal("expected length mismatch for decoded body past declared length")
	}
	if !strings.Contains(err.Error(), "length mismatch: declared 1") {
		t.Fatalf("error=%v, want declared length mismatch", err)
	}
}

func TestSuccessUntouchedByErrorMessageCap(t *testing.T) {
	large := bytes.Repeat([]byte("D"), MaxErrorMessageSize+1024)
	encoded := EncodeResponse(RespSuccess, large)
	code, decoded, err := DecodeResponse(bytes.NewReader(encoded), MaxPayloadSize)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if code != RespSuccess {
		t.Fatalf("code: expected %d, got %d", RespSuccess, code)
	}
	if !bytes.Equal(large, decoded) {
		t.Fatal("success payload should not be truncated")
	}
}

func TestDecodeResponseDoesNotAllocateDeclaredLength(t *testing.T) {
	header := append([]byte{RespSuccess}, binary.AppendUvarint(nil, MaxPayloadSize)...)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, _, err := DecodeResponse(bytes.NewReader(header), MaxPayloadSize)
	runtime.ReadMemStats(&after)

	if err == nil {
		t.Fatal("expected error for a response with no payload")
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > MaxPayloadSize/2 {
		t.Fatalf("allocated %d bytes for a %d-byte response", allocated, len(header))
	}
}

// A response chunk is read to exactly its declared length: frames decoding
// past it are refused rather than cut short, and an empty chunk, written with
// or without a stream identifier, leaves the next chunk in the stream.
func TestDecodeResponseChunkBoundaries(t *testing.T) {
	next := EncodeResponse(RespSuccess, []byte("next"))
	pastDeclared := EncodeResponse(RespSuccess, []byte("ab"))
	pastDeclared[1] = 1
	tests := []struct {
		name   string
		stream []byte
		want   []string // nil when decoding must fail
	}{
		{"two chunks", append(EncodeResponse(RespSuccess, []byte("one")), next...), []string{"one", "next"}},
		{"frames past the declared length", append(pastDeclared, next...), nil},
		{"empty chunk without a stream identifier", append([]byte{RespSuccess, 0}, next...), []string{"", "next"}},
		{"empty chunk with a stream identifier", slices.Concat([]byte{RespSuccess, 0}, []byte(streamIdentifier), next), []string{"", "next"}},
		{"empty chunk with a malformed stream identifier", slices.Concat([]byte{RespSuccess, 0}, []byte(streamIdentifier[:4]), next), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := bytes.NewReader(tc.stream)
			var got []string
			for r.Len() > 0 {
				_, data, err := DecodeResponse(r, MaxPayloadSize)
				if err != nil {
					if tc.want != nil {
						t.Fatalf("chunk %d: %v", len(got), err)
					}
					return
				}
				got = append(got, string(data))
			}
			if !slices.Equal(got, tc.want) || tc.want == nil {
				t.Fatalf("decoded %q, want %q", got, tc.want)
			}
		})
	}
}

// FuzzDecodePayload feeds arbitrary bytes to the request decoder, which must
// not panic, and whose output must survive an EncodePayload/DecodePayload
// round trip.
func FuzzDecodePayload(f *testing.F) {
	f.Add(EncodePayload([]byte("a request")))
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0x0f})
	f.Fuzz(func(t *testing.T, raw []byte) {
		data, err := DecodePayload(raw)
		if err != nil {
			return
		}
		if again, err := DecodePayload(EncodePayload(data)); err != nil || !bytes.Equal(again, data) {
			t.Fatalf("round trip of %x: got %x, %v", data, again, err)
		}
	})
}

// FuzzDecodeResponse feeds an arbitrary stream of response chunks to the
// response decoder, which must not panic.
func FuzzDecodeResponse(f *testing.F) {
	f.Add(append(EncodeResponse(RespSuccess, []byte("a block")), EncodeResponse(RespServerError, []byte("busy"))...))
	f.Add([]byte{RespSuccess, 0xff, 0xff, 0xff, 0xff, 0x0f})
	f.Fuzz(func(t *testing.T, raw []byte) {
		stream := bytes.NewReader(raw)
		for stream.Len() > 0 {
			if _, _, err := DecodeResponse(stream, 1<<16); err != nil {
				return
			}
		}
	})
}

// A response chunk carrying a block, the hot path of serving a range request.
// Blocks are mostly signatures and proofs, which do not compress.
func BenchmarkEncodeResponse(b *testing.B) {
	data := make([]byte, 256<<10)
	_, _ = rand.NewChaCha8([32]byte{}).Read(data)
	b.ReportAllocs()
	for b.Loop() {
		EncodeResponse(RespSuccess, data)
	}
}
