package gossip

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"runtime"
	"testing"

	"github.com/geanlabs/gean/internal/core/p2p/reqresp"
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

// A payload declaring more than its bytes could decode to is refused before
// the declared length is allocated, while the most compressible valid
// payload, the largest run of zeros, still decodes.
func TestDecompressBoundsTheDeclaredLength(t *testing.T) {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := Decompress(reqresp.EncodeVarint(reqresp.MaxPayloadSize))
	runtime.ReadMemStats(&after)
	if err == nil {
		t.Fatal("decoded a bare header declaring MaxPayloadSize")
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > reqresp.MaxPayloadSize/2 {
		t.Errorf("refusing a bare header allocated %d bytes", allocated)
	}

	zeros := make([]byte, reqresp.MaxPayloadSize)
	if decoded, err := Decompress(Compress(zeros)); err != nil || !bytes.Equal(decoded, zeros) {
		t.Errorf("the most compressible valid payload did not decode: %v", err)
	}
}

// FuzzDecompress feeds arbitrary bytes to gossip decompression, which must not
// panic.
func FuzzDecompress(f *testing.F) {
	f.Add(Compress([]byte("a gossip payload")))
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0x0f})
	f.Fuzz(func(t *testing.T, raw []byte) {
		_, _ = Decompress(raw)
	})
}
