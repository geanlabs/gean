package p2p

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/types"
)

type captureHandler struct {
	verdict Verdict

	mu     sync.Mutex
	blocks []*types.SignedBlock
}

func (h *captureHandler) OnBlock(block *types.SignedBlock) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.blocks = append(h.blocks, block)
}
func (h *captureHandler) OnGossipAttestation(*types.SignedAttestation)                     {}
func (h *captureHandler) OnGossipAggregatedAttestation(*types.SignedAggregatedAttestation) {}
func (h *captureHandler) ValidateGossipBlock(*types.SignedBlock) Verdict                   { return h.verdict }
func (h *captureHandler) ValidateGossipAttestation(*types.SignedAttestation) Verdict {
	return h.verdict
}
func (h *captureHandler) ValidateGossipAggregatedAttestation(*types.SignedAggregatedAttestation) Verdict {
	return h.verdict
}

func (h *captureHandler) receivedSlots() []uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	slots := make([]uint64, len(h.blocks))
	for i, b := range h.blocks {
		slots[i] = b.Block.Slot
	}
	return slots
}

func encodedGossip(t *testing.T, msg interface{ MarshalSSZ() ([]byte, error) }) []byte {
	t.Helper()
	data, err := msg.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}
	return SnappyRawEncode(data)
}

func gossipBlock(slot uint64) *types.SignedBlock {
	return &types.SignedBlock{
		Block: &types.Block{Slot: slot, Body: &types.BlockBody{}},
		Proof: &types.MultiMessageAggregate{},
	}
}

func gossipAttestation(validatorID uint64) *types.SignedAttestation {
	cp := &types.Checkpoint{}
	return &types.SignedAttestation{
		ValidatorID: validatorID,
		Data:        &types.AttestationData{Head: cp, Target: cp, Source: cp},
	}
}

func TestDecodeAndValidate(t *testing.T) {
	h := &Host{committeeCount: 4}
	var blockSizes []int
	h.Hooks.GossipBlockSize = func(n int) { blockSizes = append(blockSizes, n) }

	block := encodedGossip(t, gossipBlock(7))
	tests := []struct {
		name    string
		topic   string
		raw     []byte
		handler Verdict
		want    Verdict
		wantErr bool
	}{
		{"block passes the node's verdict", BlockTopic(), block, Ignore, Ignore, false},
		{"bad snappy", BlockTopic(), []byte{0xff, 0xff, 0xff}, Accept, Reject, true},
		{"bad ssz", BlockTopic(), SnappyRawEncode([]byte{1, 2, 3}), Accept, Reject, true},
		{"vote on its subnet", AttestationSubnetTopic(1), encodedGossip(t, gossipAttestation(5)), Accept, Accept, false},
		{"vote on another subnet", AttestationSubnetTopic(2), encodedGossip(t, gossipAttestation(5)), Accept, Reject, true},
		{"near-miss attestation topic", "/leanconsensus/12345678/not_attestation_0/ssz_snappy", block, Accept, Reject, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value, verdict, err := h.decodeAndValidate(tc.topic, tc.raw, &captureHandler{verdict: tc.handler})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v, wantErr=%t", err, tc.wantErr)
			}
			if verdict != tc.want {
				t.Fatalf("verdict=%d, want %d", verdict, tc.want)
			}
			if !tc.wantErr && value == nil {
				t.Fatal("accepted message has no decoded value")
			}
		})
	}
	if raw, _ := SnappyRawDecode(block); len(blockSizes) == 0 || blockSizes[0] != len(raw) {
		t.Fatalf("block size hook saw %v, want %d first", blockSizes, len(raw))
	}
}

func newGossipTestHost(t *testing.T) *Host {
	t.Helper()
	priv, _, err := libp2pcrypto.GenerateSecp256k1Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := priv.Raw()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "node.key")
	if err := os.WriteFile(path, []byte(hex.EncodeToString(raw)), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := NewHost(context.Background(), path, 0, 1, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	return h
}

func connectGossipHosts(t *testing.T, a, b *Host) {
	t.Helper()
	if err := a.host.Connect(context.Background(), peer.AddrInfo{ID: b.PeerID(), Addrs: b.Addrs()}); err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestValidateGossipIgnoresUntilListenersStart(t *testing.T) {
	h := newGossipTestHost(t)
	msg := &pubsub.Message{}
	if got := h.validateGossip(context.Background(), peer.ID("remote"), msg); got != pubsub.ValidationIgnore {
		t.Fatalf("verdict before listeners start=%v, want ignore", got)
	}
}

// A message the middle host rejects must not reach the far host, which is only
// connected through it. The far host's size hook runs before its own validator,
// so it records every block-topic payload that arrives, valid or not.
func TestInvalidGossipIsNotRelayed(t *testing.T) {
	origin, relay, far := newGossipTestHost(t), newGossipTestHost(t), newGossipTestHost(t)
	var mu sync.Mutex
	var arrived []int
	far.Hooks.GossipBlockSize = func(n int) {
		mu.Lock()
		defer mu.Unlock()
		arrived = append(arrived, n)
	}
	relay.StartGossipListeners(&captureHandler{verdict: Accept})
	farHandler := &captureHandler{verdict: Accept}
	far.StartGossipListeners(farHandler)
	connectGossipHosts(t, origin, relay)
	connectGossipHosts(t, relay, far)

	// Gossipsub drops a publish made before the mesh settles; probe until one
	// block crosses both hops.
	topic := BlockTopic()
	ctx := context.Background()
	waitFor(t, "a probe block to cross both hops", func() bool {
		if err := origin.topics[topic].Publish(ctx, encodedGossip(t, gossipBlock(uint64(time.Now().UnixNano())))); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
		return len(farHandler.receivedSlots()) > 0
	})

	invalid := []byte{1, 2, 3}
	if err := origin.topics[topic].Publish(ctx, SnappyRawEncode(invalid)); err != nil {
		t.Fatal(err)
	}
	if err := origin.topics[topic].Publish(ctx, encodedGossip(t, gossipBlock(9))); err != nil {
		t.Fatal(err)
	}

	// Both messages take the same path in order, so once the valid block is in,
	// a relayed invalid one would already have arrived.
	waitFor(t, "the valid block at the far host", func() bool {
		slots := farHandler.receivedSlots()
		return len(slots) > 0 && slots[len(slots)-1] == 9
	})
	mu.Lock()
	defer mu.Unlock()
	for _, n := range arrived {
		if n == len(invalid) {
			t.Fatalf("invalid payload reached the far host: arrivals %v", arrived)
		}
	}
}
