package gossip

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"testing"
)

// Any topic strings do; these have the shape of real ones.
const (
	blockTopic       = "/leanconsensus/12345678/block/ssz_snappy"
	aggregationTopic = "/leanconsensus/12345678/aggregation/ssz_snappy"
)

func TestSnappyRawRoundtrip(t *testing.T) {
	data := []byte("hello lean consensus world")
	compressed := Compress(data)
	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(data, decompressed) {
		t.Fatal("roundtrip mismatch")
	}
}

func TestComputeMessageIDDeterministic(t *testing.T) {
	topic := blockTopic
	data := Compress([]byte("block data"))

	id1 := MessageID(topic, data)
	id2 := MessageID(topic, data)

	if !bytes.Equal(id1, id2) {
		t.Fatal("message IDs should be deterministic")
	}
	if len(id1) != 20 {
		t.Fatalf("message ID should be 20 bytes, got %d", len(id1))
	}
}

func TestComputeMessageIDDifferentTopics(t *testing.T) {
	data := Compress([]byte("same data"))
	id1 := MessageID(blockTopic, data)
	id2 := MessageID(aggregationTopic, data)

	if bytes.Equal(id1, id2) {
		t.Fatal("different topics should produce different IDs")
	}
}

func TestComputeMessageIDInvalidSnappy(t *testing.T) {
	id := MessageID(blockTopic, []byte{0xff, 0xfe, 0xfd})
	if len(id) != 20 {
		t.Fatalf("should still produce 20-byte ID, got %d", len(id))
	}
}

func TestComputeMessageIDMatchesWireFormula(t *testing.T) {
	topic := blockTopic
	payload := []byte("block data")
	raw := Compress(payload)

	got := MessageID(topic, raw)
	want := messageIDDigest(domainValidSnappy, topic, payload)
	if !bytes.Equal(got, want) {
		t.Fatalf("valid snappy message ID=%x, want %x", got, want)
	}

	invalidRaw := []byte{0xff, 0xfe, 0xfd}
	got = MessageID(topic, invalidRaw)
	want = messageIDDigest(domainInvalidSnappy, topic, invalidRaw)
	if !bytes.Equal(got, want) {
		t.Fatalf("invalid snappy message ID=%x, want %x", got, want)
	}
}

func messageIDDigest(domain [4]byte, topic string, data []byte) []byte {
	h := sha256.New()
	var topicLen [8]byte
	topicBytes := []byte(topic)
	binary.LittleEndian.PutUint64(topicLen[:], uint64(len(topicBytes)))
	h.Write(domain[:])
	h.Write(topicLen[:])
	h.Write(topicBytes)
	h.Write(data)
	return h.Sum(nil)[:20]
}
