package p2p

import (
	"bytes"
	"os"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

// --- Encoding tests ---

func TestSnappyRawRoundtrip(t *testing.T) {
	data := []byte("hello lean consensus world")
	compressed := SnappyRawEncode(data)
	decompressed, err := SnappyRawDecode(compressed)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(data, decompressed) {
		t.Fatal("roundtrip mismatch")
	}
}

func TestVarintRoundtrip(t *testing.T) {
	tests := []uint32{0, 1, 127, 128, 255, 256, 16383, 16384, 1<<20 - 1}
	for _, v := range tests {
		encoded := EncodeVarint(v)
		decoded, rest, err := DecodeVarint(encoded)
		if err != nil {
			t.Fatalf("varint %d: %v", v, err)
		}
		if decoded != v {
			t.Fatalf("varint %d: got %d", v, decoded)
		}
		if len(rest) != 0 {
			t.Fatalf("varint %d: %d trailing bytes", v, len(rest))
		}
	}
}

func TestReqRespPayloadRoundtrip(t *testing.T) {
	data := []byte("state transition function data")
	encoded := EncodeReqRespPayload(data)
	decoded, err := DecodeReqRespPayload(encoded)
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
	code, decoded, err := DecodeResponse(reader)
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

// --- Topic tests ---

func TestTopicStrings(t *testing.T) {
	if BlockTopic() != "/leanconsensus/devnet0/block/ssz_snappy" {
		t.Fatalf("block topic: %s", BlockTopic())
	}
	if AggregationTopic() != "/leanconsensus/devnet0/aggregation/ssz_snappy" {
		t.Fatalf("aggregation topic: %s", AggregationTopic())
	}
	if AttestationSubnetTopic(0) != "/leanconsensus/devnet0/attestation_0/ssz_snappy" {
		t.Fatalf("attestation subnet 0: %s", AttestationSubnetTopic(0))
	}
	if AttestationSubnetTopic(3) != "/leanconsensus/devnet0/attestation_3/ssz_snappy" {
		t.Fatalf("attestation subnet 3: %s", AttestationSubnetTopic(3))
	}
}

func TestSubnetID(t *testing.T) {
	if SubnetID(0, 1) != 0 {
		t.Fatal("subnet 0%1 != 0")
	}
	if SubnetID(5, 3) != 2 {
		t.Fatal("subnet 5%3 != 2")
	}
	if SubnetID(7, 4) != 3 {
		t.Fatal("subnet 7%4 != 3")
	}
}

// --- Message ID tests ---

func TestComputeMessageIDDeterministic(t *testing.T) {
	topic := BlockTopic()
	data := SnappyRawEncode([]byte("block data"))

	id1 := ComputeMessageID(topic, data)
	id2 := ComputeMessageID(topic, data)

	if !bytes.Equal(id1, id2) {
		t.Fatal("message IDs should be deterministic")
	}
	if len(id1) != 20 {
		t.Fatalf("message ID should be 20 bytes, got %d", len(id1))
	}
}

func TestComputeMessageIDDifferentTopics(t *testing.T) {
	data := SnappyRawEncode([]byte("same data"))
	id1 := ComputeMessageID(BlockTopic(), data)
	id2 := ComputeMessageID(AggregationTopic(), data)

	if bytes.Equal(id1, id2) {
		t.Fatal("different topics should produce different IDs")
	}
}

func TestComputeMessageIDInvalidSnappy(t *testing.T) {
	id := ComputeMessageID(BlockTopic(), []byte{0xff, 0xfe, 0xfd})
	if len(id) != 20 {
		t.Fatalf("should still produce 20-byte ID, got %d", len(id))
	}
}

// --- Status message tests ---

func TestStatusMessageSSZRoundtrip(t *testing.T) {
	status := &StatusMessage{
		FinalizedRoot: [32]byte{0xab},
		FinalizedSlot: 42,
		HeadRoot:      [32]byte{0xcd},
		HeadSlot:      100,
	}
	data := status.MarshalSSZ()
	if len(data) != 80 {
		t.Fatalf("status SSZ should be 80 bytes, got %d", len(data))
	}

	decoded := &StatusMessage{}
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.FinalizedSlot != 42 || decoded.HeadSlot != 100 {
		t.Fatal("status roundtrip mismatch")
	}
	if decoded.FinalizedRoot != status.FinalizedRoot {
		t.Fatal("finalized root mismatch")
	}
	if decoded.HeadRoot != status.HeadRoot {
		t.Fatal("head root mismatch")
	}
}

func TestBlocksByRootRequestSSZRoundtrip(t *testing.T) {
	roots := [][32]byte{
		{0x01, 0x02, 0x03},
		{0xaa, 0xbb, 0xcc},
	}
	encoded := EncodeBlocksByRootRequest(roots)

	// SSZ container: 4-byte offset + 2 * 32 bytes = 68 bytes.
	if len(encoded) != 68 {
		t.Fatalf("expected 68 bytes, got %d", len(encoded))
	}
	// First 4 bytes should be offset = 4 (little-endian).
	if encoded[0] != 4 || encoded[1] != 0 || encoded[2] != 0 || encoded[3] != 0 {
		t.Fatalf("unexpected offset bytes: %v", encoded[:4])
	}

	decoded, err := DecodeBlocksByRootRequest(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(decoded))
	}
	if decoded[0] != roots[0] || decoded[1] != roots[1] {
		t.Fatal("root mismatch")
	}
}

func TestBlocksByRootRequestSingleRoot36Bytes(t *testing.T) {
	// Simulate what other clients send: 1 root = 36 bytes (4-byte offset + 32 bytes).
	roots := [][32]byte{{0xde, 0xad, 0xbe, 0xef}}
	encoded := EncodeBlocksByRootRequest(roots)
	if len(encoded) != 36 {
		t.Fatalf("expected 36 bytes for single root, got %d", len(encoded))
	}

	decoded, err := DecodeBlocksByRootRequest(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded) != 1 || decoded[0] != roots[0] {
		t.Fatal("single root roundtrip mismatch")
	}
}

// --- Peer store tests ---

func TestPeerStoreAddRemove(t *testing.T) {
	ps := NewPeerStore()
	if ps.Count() != 0 {
		t.Fatal("should start empty")
	}

	ps.Add("peer1")
	ps.Add("peer2")
	if ps.Count() != 2 {
		t.Fatalf("expected 2, got %d", ps.Count())
	}

	ps.Remove("peer1")
	if ps.Count() != 1 {
		t.Fatalf("expected 1, got %d", ps.Count())
	}
}

func TestPeerStoreRandomPeer(t *testing.T) {
	ps := NewPeerStore()
	ps.Add("peer1")
	ps.Add("peer2")
	ps.Add("peer3")

	exclude := map[peer.ID]bool{"peer1": true, "peer2": true}
	p := ps.RandomPeer(exclude)
	if p != "peer3" {
		t.Fatalf("expected peer3, got %s", p)
	}
}

func TestPeerStoreRandomPeerNoneAvailable(t *testing.T) {
	ps := NewPeerStore()
	p := ps.RandomPeer(nil)
	if p != "" {
		t.Fatal("should return empty when no peers")
	}
}

// --- Bootnode loading ---

func TestLoadBootnodesEmpty(t *testing.T) {
	tmpFile := t.TempDir() + "/nodes.yaml"
	os.WriteFile(tmpFile, []byte("# empty\n"), 0644)

	addrs, err := LoadBootnodes(tmpFile)
	if err != nil {
		t.Fatalf("should not error on empty: %v", err)
	}
	if len(addrs) != 0 {
		t.Fatalf("expected 0 addrs, got %d", len(addrs))
	}
}

func TestLoadBootnodesWithYAMLList(t *testing.T) {
	content := `- "/ip4/127.0.0.1/udp/9000/quic-v1/p2p/12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN"
- "/ip4/127.0.0.1/udp/9001/quic-v1/p2p/12D3KooWLc4yBi3vYo4udihGu2HFxCWMWCdJoXYMFNp2CX9otY5A"
`
	tmpFile := t.TempDir() + "/nodes.yaml"
	os.WriteFile(tmpFile, []byte(content), 0644)

	addrs, err := LoadBootnodes(tmpFile)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addrs, got %d", len(addrs))
	}
}
