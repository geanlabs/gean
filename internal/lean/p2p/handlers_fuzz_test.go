package p2p

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"

	"github.com/geanlabs/gean/internal/core/p2p/reqresp"
	"github.com/geanlabs/gean/internal/lean/types"
)

// handlerStream is a stream whose peer sent request and reads the response. The
// request handlers serve any peer, so each fuzz target feeds one arbitrary
// request bytes and checks the store is asked only for validated requests and
// the response serves exactly what was asked for.
type handlerStream struct {
	network.Stream
	request  *bytes.Reader
	response bytes.Buffer
}

func newHandlerStream(request []byte) *handlerStream {
	return &handlerStream{request: bytes.NewReader(request)}
}

func (s *handlerStream) Read(p []byte) (int, error)     { return s.request.Read(p) }
func (s *handlerStream) Write(p []byte) (int, error)    { return s.response.Write(p) }
func (*handlerStream) SetReadDeadline(time.Time) error  { return nil }
func (*handlerStream) SetWriteDeadline(time.Time) error { return nil }

// chunks decodes the response, failing the test on a malformed chunk or code.
func (s *handlerStream) chunks(t *testing.T) (codes []byte, payloads [][]byte) {
	t.Helper()
	for {
		code, payload, err := reqresp.DecodeResponse(&s.response, MaxSignedBlockSize)
		if errors.Is(err, io.EOF) {
			return codes, payloads
		}
		if err != nil {
			t.Fatalf("response chunk %d: %v", len(codes), err)
		}
		if code > reqresp.RespResourceUnavailable {
			t.Fatalf("response chunk %d: code %d", len(codes), code)
		}
		codes, payloads = append(codes, code), append(payloads, payload)
	}
}

// servedBlock decodes a success chunk as a block.
func servedBlock(t *testing.T, payload []byte) *types.SignedBlock {
	t.Helper()
	block := &types.SignedBlock{}
	if err := block.UnmarshalSSZ(payload); err != nil {
		t.Fatalf("served block: %v", err)
	}
	return block
}

// fuzzChainLength is how many blocks, one per slot from 0, the fake store holds.
const fuzzChainLength = 64

func FuzzBlocksByRangeHandler(f *testing.F) {
	chain := make([]*types.SignedBlock, fuzzChainLength)
	for slot := range chain {
		chain[slot], _ = signedBlockForP2PTest(f, uint64(slot))
	}
	rangeRequest := func(start, count uint64) []byte {
		ssz, err := (&types.BlocksByRangeRequest{StartSlot: start, Count: count}).MarshalSSZ()
		if err != nil {
			f.Fatal(err)
		}
		return reqresp.EncodePayload(ssz)
	}
	f.Add(rangeRequest(3, 5), uint64(10), true)
	f.Add(rangeRequest(60, 10), uint64(70), false)
	f.Add(rangeRequest(0, types.MaxRequestBlocks+1), uint64(0), true)

	f.Fuzz(func(t *testing.T, request []byte, currentSlot uint64, complete bool) {
		var asked *types.BlocksByRangeRequest
		var returned []*types.SignedBlock
		stream := newHandlerStream(request)
		handleBlocksByRangeRequest(stream, func() uint64 { return currentSlot }, func(start, count uint64) ([]*types.SignedBlock, bool) {
			if count == 0 || count > types.MaxRequestBlocks || start+count < start {
				t.Fatalf("store asked for %d blocks from slot %d", count, start)
			}
			if currentSlot > types.MinSlotsForBlockRequests && start < currentSlot-types.MinSlotsForBlockRequests {
				t.Fatalf("store asked for slot %d, below the history window at slot %d", start, currentSlot)
			}
			asked = &types.BlocksByRangeRequest{StartSlot: start, Count: count}
			if end := min(start+count, fuzzChainLength); start < end {
				returned = chain[start:end]
			}
			return returned, complete
		})

		codes, payloads := stream.chunks(t)
		if asked == nil || !complete {
			if len(codes) != 1 || codes[0] == reqresp.RespSuccess {
				t.Fatalf("refused request answered with codes %v, want one error", codes)
			}
			return
		}
		for i, code := range codes {
			if code != reqresp.RespSuccess {
				t.Fatalf("chunk %d: code %d serving held blocks", i, code)
			}
			if slot := servedBlock(t, payloads[i]).Block.Slot; slot != asked.StartSlot+uint64(i) {
				t.Fatalf("chunk %d: slot %d, want %d", i, slot, asked.StartSlot+uint64(i))
			}
		}
		if len(codes) != len(returned) {
			t.Fatalf("served %d of the %d blocks held", len(codes), len(returned))
		}
	})
}

func FuzzBlocksByRootHandler(f *testing.F) {
	byRoot := make(map[[32]byte]*types.SignedBlock, fuzzChainLength)
	roots := make([][32]byte, fuzzChainLength)
	for slot := range roots {
		var block *types.SignedBlock
		block, roots[slot] = signedBlockForP2PTest(f, uint64(slot))
		byRoot[roots[slot]] = block
	}
	rootRequest := func(roots ...[32]byte) []byte {
		ssz, err := (&types.BlocksByRootRequest{Roots: roots}).MarshalSSZ()
		if err != nil {
			f.Fatal(err)
		}
		return reqresp.EncodePayload(ssz)
	}
	f.Add(rootRequest(roots[1], [32]byte{0xff}, roots[7]))
	f.Add(rootRequest())

	f.Fuzz(func(t *testing.T, request []byte) {
		var lookups int
		var held [][32]byte
		stream := newHandlerStream(request)
		handleBlocksByRootRequest(stream, func(root [32]byte) *types.SignedBlock {
			if lookups++; lookups > types.MaxRequestBlocks {
				t.Fatalf("store asked for more than %d roots", types.MaxRequestBlocks)
			}
			if byRoot[root] != nil {
				held = append(held, root)
			}
			return byRoot[root]
		})

		codes, payloads := stream.chunks(t)
		if lookups == 0 {
			if len(codes) != 1 || codes[0] == reqresp.RespSuccess {
				t.Fatalf("refused request answered with codes %v, want one error", codes)
			}
			return
		}
		if len(codes) != len(held) {
			t.Fatalf("served %d of the %d requested blocks held", len(codes), len(held))
		}
		for i, code := range codes {
			if code != reqresp.RespSuccess {
				t.Fatalf("chunk %d: code %d serving held blocks", i, code)
			}
			root, err := servedBlock(t, payloads[i]).Block.HashTreeRoot()
			if err != nil {
				t.Fatal(err)
			}
			if root != held[i] {
				t.Fatalf("chunk %d: served %x, want the requested %x", i, root, held[i])
			}
		}
	})
}

func FuzzStatusHandler(f *testing.F) {
	ours := &StatusMessage{FinalizedSlot: 4, HeadSlot: 9}
	f.Add(reqresp.EncodePayload((&StatusMessage{HeadSlot: 3}).MarshalSSZ()), true)
	f.Add([]byte{}, true)

	f.Fuzz(func(t *testing.T, request []byte, available bool) {
		stream := newHandlerStream(request)
		theirs := handleStatusRequest(stream, func() *StatusMessage {
			if !available {
				return nil
			}
			return ours
		})

		codes, payloads := stream.chunks(t)
		if len(codes) != 1 {
			t.Fatalf("status answered with %d chunks", len(codes))
		}
		if (codes[0] == reqresp.RespSuccess) != (theirs != nil) {
			t.Fatalf("status answered with code %d, peer status returned %v", codes[0], theirs != nil)
		}
		if theirs != nil && !bytes.Equal(payloads[0], ours.MarshalSSZ()) {
			t.Fatalf("status answered with %x, want ours", payloads[0])
		}
	})
}
