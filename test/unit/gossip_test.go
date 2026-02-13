package unit

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/golang/snappy"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"

	"github.com/geanlabs/gean/network/gossipsub"
)

func TestComputeMessageID(t *testing.T) {
	topicStr := "/leanconsensus/devnet0/block/ssz_snappy"
	data := []byte("test data")

	// Snappy block-encode so ComputeMessageID's Decode succeeds (valid domain).
	compressed := snappy.Encode(nil, data)

	// Expected: SHA256(DomainValidSnappy + le64(topicLen) + topic + decompressedData)[:20]
	var topicLen [8]byte
	binary.LittleEndian.PutUint64(topicLen[:], uint64(len(topicStr)))

	h := sha256.New()
	h.Write(gossipsub.DomainValidSnappy)
	h.Write(topicLen[:])
	h.Write([]byte(topicStr))
	h.Write(data)
	expected := string(h.Sum(nil)[:20])

	msg := &pb.Message{
		Topic: &topicStr,
		Data:  compressed,
	}

	got := gossipsub.ComputeMessageID(msg)
	if got != expected {
		t.Fatalf("ComputeMessageID mismatch:\n  got:    %x\n  expect: %x", []byte(got), []byte(expected))
	}
}

func TestComputeMessageIDInvalidSnappy(t *testing.T) {
	topicStr := "/leanconsensus/devnet0/block/ssz_snappy"
	data := []byte("not valid snappy data")

	// Expected: SHA256(DomainInvalidSnappy + le64(topicLen) + topic + rawData)[:20]
	var topicLen [8]byte
	binary.LittleEndian.PutUint64(topicLen[:], uint64(len(topicStr)))

	h := sha256.New()
	h.Write(gossipsub.DomainInvalidSnappy)
	h.Write(topicLen[:])
	h.Write([]byte(topicStr))
	h.Write(data)
	expected := string(h.Sum(nil)[:20])

	msg := &pb.Message{
		Topic: &topicStr,
		Data:  data,
	}

	got := gossipsub.ComputeMessageID(msg)
	if got != expected {
		t.Fatalf("ComputeMessageID mismatch for invalid snappy:\n  got:    %x\n  expect: %x", []byte(got), []byte(expected))
	}
}
