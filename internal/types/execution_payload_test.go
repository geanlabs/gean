package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"testing"
)

func samplePayload() *ExecutionPayload {
	p := &ExecutionPayload{
		ParentHash:    [32]byte{0x11},
		FeeRecipient:  [AddressSize]byte{0x22},
		StateRoot:     [32]byte{0x33},
		ReceiptsRoot:  [32]byte{0x44},
		PrevRandao:    [32]byte{0x55},
		BlockNumber:   7,
		GasLimit:      30_000_000,
		GasUsed:       21_000,
		Timestamp:     1_700_000_004,
		ExtraData:     []byte("gean"),
		BaseFeePerGas: [32]byte{0x07},
		BlockHash:     [32]byte{0x66},
		Transactions:  [][]byte{{0x02, 0xf8, 0x01}, bytes.Repeat([]byte{0xab}, 100)},
		Withdrawals:   []*Withdrawal{{Index: 1, ValidatorIndex: 2, Address: [AddressSize]byte{0x99}, Amount: 3}},
		BlobGasUsed:   0,
		ExcessBlobGas: 0,
	}
	p.LogsBloom[0] = 0x01
	return p
}

func TestExecutionPayloadRoundTrip(t *testing.T) {
	want := samplePayload()
	data, err := want.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := &ExecutionPayload{}
	if err := got.UnmarshalSSZ(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	again, err := got.MarshalSSZ()
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if !bytes.Equal(data, again) {
		t.Fatal("ssz round trip changed the encoding")
	}
	wantRoot, _ := want.HashTreeRoot()
	gotRoot, _ := got.HashTreeRoot()
	if wantRoot != gotRoot {
		t.Fatal("ssz round trip changed the root")
	}
}

func TestExecutionPayloadIsZero(t *testing.T) {
	var nilPayload *ExecutionPayload
	if !nilPayload.IsZero() {
		t.Fatal("nil payload should be zero")
	}
	if !(&ExecutionPayload{}).IsZero() {
		t.Fatal("empty payload should be zero")
	}
	if (&ExecutionPayload{Timestamp: 1}).IsZero() {
		t.Fatal("payload with a timestamp is not zero")
	}
	if (&ExecutionPayload{Transactions: [][]byte{{}}}).IsZero() {
		t.Fatal("payload with an empty transaction is not zero")
	}
	// A body built without a payload is the zero payload, which is what a
	// pure-consensus network expects every block to carry.
	if !(&BlockBody{}).ExecutionPayload.IsZero() {
		t.Fatal("a body built without a payload should carry the zero payload")
	}
}

// The header's list roots are computed by hand because sszgen only emits
// container hashers. Check them against an independent merkleization so a
// mistake in the hand-rolled path cannot pass by agreeing with itself.
func TestExecutionPayloadHeaderListRoots(t *testing.T) {
	p := samplePayload()
	h, err := p.ToHeader()
	if err != nil {
		t.Fatalf("to header: %v", err)
	}

	txChunks := make([][32]byte, len(p.Transactions))
	for i, tx := range p.Transactions {
		txChunks[i] = refMixInLength(refMerkleize(refPackBytes(tx), (MaxBytesPerTransaction+31)/32), uint64(len(tx)))
	}
	if want := refMixInLength(refMerkleize(txChunks, MaxTransactionsPerPayload), uint64(len(p.Transactions))); h.TransactionsRoot != want {
		t.Fatalf("transactions root: got %x want %x", h.TransactionsRoot, want)
	}

	wChunks := make([][32]byte, len(p.Withdrawals))
	for i, w := range p.Withdrawals {
		root, err := w.HashTreeRoot()
		if err != nil {
			t.Fatalf("withdrawal root: %v", err)
		}
		wChunks[i] = root
	}
	if want := refMixInLength(refMerkleize(wChunks, MaxWithdrawalsPerPayload), uint64(len(p.Withdrawals))); h.WithdrawalsRoot != want {
		t.Fatalf("withdrawals root: got %x want %x", h.WithdrawalsRoot, want)
	}

	if h.BlockHash != p.BlockHash || h.Timestamp != p.Timestamp || !bytes.Equal(h.ExtraData, p.ExtraData) {
		t.Fatal("header did not copy scalar fields")
	}
	h.ExtraData[0] = 'x'
	if p.ExtraData[0] == 'x' {
		t.Fatal("header shares extra data with the payload")
	}
}

func TestExecutionPayloadListBounds(t *testing.T) {
	if _, err := WithdrawalsRoot(make([]*Withdrawal, MaxWithdrawalsPerPayload+1)); err == nil {
		t.Fatal("expected withdrawals over the limit to be rejected")
	}
	if _, err := TransactionsRoot([][]byte{make([]byte, MaxBytesPerTransaction+1)}); err == nil {
		t.Fatal("expected an oversized transaction to be rejected")
	}
}

// Reference SSZ merkleization, kept deliberately naive.

func refPackBytes(b []byte) [][32]byte {
	chunks := make([][32]byte, (len(b)+31)/32)
	for i := range chunks {
		copy(chunks[i][:], b[i*32:])
	}
	return chunks
}

func refMerkleize(chunks [][32]byte, limit uint64) [32]byte {
	depth := 0
	for uint64(1)<<depth < limit {
		depth++
	}
	zero := make([][32]byte, depth+1)
	for i := 1; i <= depth; i++ {
		zero[i] = refHashPair(zero[i-1], zero[i-1])
	}
	layer := append([][32]byte(nil), chunks...)
	for level := 0; level < depth; level++ {
		if len(layer)%2 == 1 {
			layer = append(layer, zero[level])
		}
		next := make([][32]byte, 0, len(layer)/2)
		for i := 0; i < len(layer); i += 2 {
			next = append(next, refHashPair(layer[i], layer[i+1]))
		}
		layer = next
		if len(layer) == 0 {
			layer = [][32]byte{zero[level+1]}
		}
	}
	if len(layer) == 0 {
		return zero[depth]
	}
	return layer[0]
}

func refMixInLength(root [32]byte, length uint64) [32]byte {
	var lengthChunk [32]byte
	binary.LittleEndian.PutUint64(lengthChunk[:], length)
	return refHashPair(root, lengthChunk)
}

func refHashPair(a, b [32]byte) [32]byte {
	return sha256.Sum256(append(a[:], b[:]...))
}
