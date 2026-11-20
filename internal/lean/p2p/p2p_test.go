package p2p

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/core/p2p/gossip"
	"github.com/geanlabs/gean/internal/core/p2p/host"
	"github.com/geanlabs/gean/internal/core/p2p/memnet"
	"github.com/geanlabs/gean/internal/core/p2p/peers"
	"github.com/geanlabs/gean/internal/core/p2p/reqresp"
	"github.com/geanlabs/gean/internal/lean/types"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestLogPublishIncludesDevnetDiagnostics(t *testing.T) {
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetQuiet(false)
	t.Cleanup(func() {
		logger.SetOutput(os.Stderr)
		SetClientGitCommit(unknownBuildValue)
	})

	SetClientGitCommit("abc123")
	sszData := []byte("signed block ssz")
	compressed := gossip.Compress(sszData)
	root := [32]byte{0xaa}
	logPublish(BlockTopic(), sszData, compressed, publishLogInfo{
		slot:         7,
		proposer:     "3",
		blockRoot:    root,
		hasBlockRoot: true,
	})

	out := buf.String()
	wantParts := []string{
		"publish gossip",
		"topic=" + BlockTopic(),
		"slot=7",
		"proposer=3",
		"block_root=0xaa",
		"sha256_ssz=0x",
		"sha256_compressed=0x",
		"compressed_len=",
		"snappy_self_decode_ok=true",
		"message_id=0x",
		"client_git_sha=abc123",
		"snappy=github.com/golang/snappy@",
	}
	for _, part := range wantParts {
		if !strings.Contains(out, part) {
			t.Fatalf("publish log missing %q:\n%s", part, out)
		}
	}
}

func TestPublishBlockLogsCanonicalBlockRoot(t *testing.T) {
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetQuiet(false)
	t.Cleanup(func() {
		logger.SetOutput(os.Stderr)
	})

	signed := &types.SignedBlock{
		Block: &types.Block{
			Slot:          7,
			ProposerIndex: 3,
			Body:          &types.BlockBody{},
		},
		Proof: &types.MultiMessageAggregate{Proof: []byte{0x42}},
	}
	blockRoot, err := signed.Block.HashTreeRoot()
	if err != nil {
		t.Fatalf("block root: %v", err)
	}
	signedRoot, err := signed.HashTreeRoot()
	if err != nil {
		t.Fatalf("signed block root: %v", err)
	}
	if blockRoot == signedRoot {
		t.Fatal("test setup failed: signed block root unexpectedly equals canonical block root")
	}

	h := &Host{Host: &host.Host{}}
	err = h.PublishBlock(context.Background(), signed)
	if err == nil || !strings.Contains(err.Error(), "not subscribed to topic") {
		t.Fatalf("PublishBlock error=%v, want missing topic after log", err)
	}

	out := buf.String()
	want := "block_root=0x" + hex.EncodeToString(blockRoot[:])
	if !strings.Contains(out, want) {
		t.Fatalf("publish log missing canonical root %s:\n%s", want, out)
	}
	wrong := "block_root=0x" + hex.EncodeToString(signedRoot[:])
	if strings.Contains(out, wrong) {
		t.Fatalf("publish log used signed-block root %s:\n%s", wrong, out)
	}
}

func TestDecodeResponseMaxSignedBlockSize(t *testing.T) {
	attestations := make([]*types.AggregatedAttestation, types.ValidatorRegistryLimit)
	for i := range attestations {
		bits := make([]byte, types.ValidatorRegistryLimit/8+1)
		bits[len(bits)-1] = 1
		attestations[i] = &types.AggregatedAttestation{
			AggregationBits: bits,
			Data:            &types.AttestationData{Head: &types.Checkpoint{}, Target: &types.Checkpoint{}, Source: &types.Checkpoint{}},
		}
	}
	block := &types.SignedBlock{
		Block: &types.Block{Body: &types.BlockBody{Attestations: attestations}},
		Proof: &types.MultiMessageAggregate{Proof: make([]byte, 512<<10)},
	}
	encoded, err := block.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal max block: %v", err)
	}
	if len(encoded) != MaxSignedBlockSize {
		t.Fatalf("max block is %d bytes, MaxSignedBlockSize is %d", len(encoded), MaxSignedBlockSize)
	}

	if _, _, err := reqresp.DecodeResponse(bytes.NewReader(reqresp.EncodeResponse(reqresp.RespSuccess, encoded)), MaxSignedBlockSize); err != nil {
		t.Fatalf("decode max block: %v", err)
	}
	oversized := reqresp.EncodeResponse(reqresp.RespSuccess, append(encoded, 0))
	if _, _, err := reqresp.DecodeResponse(bytes.NewReader(oversized), MaxSignedBlockSize); err == nil {
		t.Fatal("expected error for a chunk one byte over MaxSignedBlockSize")
	}
}

func TestPublishRejectsNilMessages(t *testing.T) {
	h := &Host{Host: &host.Host{}}
	ctx := context.Background()

	if err := h.PublishBlock(ctx, nil); err == nil {
		t.Fatal("expected nil block error")
	}
	if err := h.PublishAttestation(ctx, nil, 1); err == nil {
		t.Fatal("expected nil attestation error")
	}
	if err := h.PublishAggregatedAttestation(ctx, nil); err == nil {
		t.Fatal("expected nil aggregation error")
	}
}

func TestTopicStrings(t *testing.T) {
	if BlockTopic() != "/leanconsensus/12345678/block/ssz_snappy" {
		t.Fatalf("block topic: %s", BlockTopic())
	}
	if AggregationTopic() != "/leanconsensus/12345678/aggregation/ssz_snappy" {
		t.Fatalf("aggregation topic: %s", AggregationTopic())
	}
	if AttestationSubnetTopic(0) != "/leanconsensus/12345678/attestation_0/ssz_snappy" {
		t.Fatalf("attestation subnet 0: %s", AttestationSubnetTopic(0))
	}
	if AttestationSubnetTopic(3) != "/leanconsensus/12345678/attestation_3/ssz_snappy" {
		t.Fatalf("attestation subnet 3: %s", AttestationSubnetTopic(3))
	}
}

func TestAttestationSubnet(t *testing.T) {
	tests := []struct {
		topic  string
		subnet uint64
		ok     bool
	}{
		{AttestationSubnetTopic(0), 0, true},
		{AttestationSubnetTopic(42), 42, true},
		{"/leanconsensus/12345678/attestation_/ssz_snappy", 0, false},
		{"/leanconsensus/12345678/attestation_x/ssz_snappy", 0, false},
		{"/leanconsensus/12345678/not_attestation_0/ssz_snappy", 0, false},
		{"/other/12345678/attestation_0/ssz_snappy", 0, false},
		{"/leanconsensus/12345678/attestation_0/json", 0, false},
	}

	for _, test := range tests {
		if subnet, ok := attestationSubnet(test.topic); subnet != test.subnet || ok != test.ok {
			t.Fatalf("attestationSubnet(%q)=(%d,%t), want (%d,%t)", test.topic, subnet, ok, test.subnet, test.ok)
		}
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
	if SubnetID(7, 0) != 0 {
		t.Fatal("subnet with zero committees should return 0")
	}
}

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

	if len(encoded) != 68 {
		t.Fatalf("expected 68 bytes, got %d", len(encoded))
	}
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

func TestValidateFetchedBlockRootRejectsUnrequestedAndDuplicateBlocks(t *testing.T) {
	requestedBlock, requestedRoot := signedBlockForP2PTest(t, 1)
	otherBlock, _ := signedBlockForP2PTest(t, 2)

	requestedRoots := requestedRootSet([][32]byte{requestedRoot})
	seenRoots := make(map[[32]byte]bool)
	if err := validateFetchedBlockRoot(requestedBlock, requestedRoots, seenRoots); err != nil {
		t.Fatalf("requested block rejected: %v", err)
	}
	if err := validateFetchedBlockRoot(requestedBlock, requestedRoots, seenRoots); err == nil {
		t.Fatal("expected duplicate block root rejection")
	}
	if err := validateFetchedBlockRoot(otherBlock, requestedRoots, make(map[[32]byte]bool)); err == nil {
		t.Fatal("expected unrequested block root rejection")
	}
}

func signedBlockForP2PTest(t *testing.T, slot uint64) (*types.SignedBlock, [32]byte) {
	t.Helper()

	block := &types.Block{
		Slot: slot,
		Body: &types.BlockBody{},
	}
	root, err := block.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash test block: %v", err)
	}
	return &types.SignedBlock{
		Block: block,
		Proof: &types.MultiMessageAggregate{},
	}, root
}

// End to end over libp2p: a refused range request costs the serving peer score,
// a served one earns it, and both sides of a status exchange record the head the
// other reported.
func TestHostScoresRequestsAndRecordsStatus(t *testing.T) {
	block, root := signedBlockForP2PTest(t, 1)
	serve := func(h *Host, complete bool) {
		h.RegisterReqRespHandlers(
			func() *StatusMessage { return &StatusMessage{HeadSlot: 1, HeadRoot: root} },
			func([32]byte) *types.SignedBlock { return nil },
			func() uint64 { return 1 },
			func(uint64, uint64) ([]*types.SignedBlock, bool) {
				return []*types.SignedBlock{block}, complete
			},
		)
	}
	client := newTestHost(t, nil)
	serve(client, true)
	refusing, serving := newTestHost(t, nil), newTestHost(t, nil)
	serve(refusing, false)
	serve(serving, true)

	connectTestHosts(t, client, refusing, serving)
	waitFor(t, "the refusing peer to register the client", func() bool { return refusing.ConnectedPeers() >= 1 })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := client.FetchBlocksByRange(ctx, refusing.PeerID(), 1, 1); err == nil {
		t.Fatal("refusing peer served the range")
	}
	if got := peerScore(client.PeerStore(), refusing.PeerID()); got != peers.InitialScore-peers.ScoreFailure {
		t.Fatalf("refusing peer score = %d, want %d", got, peers.InitialScore-peers.ScoreFailure)
	}
	if _, err := client.FetchBlocksByRange(ctx, serving.PeerID(), 1, 1); err != nil {
		t.Fatalf("serving peer: %v", err)
	}
	if got := peerScore(client.PeerStore(), serving.PeerID()); got != peers.InitialScore+peers.ScoreSuccess {
		t.Fatalf("serving peer score = %d, want %d", got, peers.InitialScore+peers.ScoreSuccess)
	}

	if _, err := client.SendStatusRequest(ctx, refusing.PeerID(), &StatusMessage{HeadSlot: 7}); err != nil {
		t.Fatal(err)
	}
	if !headRecorded(client.PeerStore(), refusing.PeerID(), 1) {
		t.Fatal("outbound status response was not recorded")
	}
	if !headRecorded(refusing.PeerStore(), client.PeerID(), 7) {
		t.Fatal("inbound status request was not recorded")
	}
}

func peerScore(ps *peers.Store, id peer.ID) int {
	score, _ := ps.Score(id)
	return score
}

func headRecorded(ps *peers.Store, id peer.ID, head uint64) bool {
	got, ok := ps.HeadSlot(id)
	return ok && got == head
}

// newTestHost starts a host with a fresh key, on QUIC or, when n is not nil, on
// the in-memory network n.
func newTestHost(t *testing.T, n *memnet.Network) *Host {
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
	cfg := host.Config{NodeKeyPath: path}
	if n != nil {
		cfg.Network = n
	}
	h, err := NewHost(context.Background(), cfg, 1, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	return h
}

// connectTestHosts connects client to each server and waits until the client
// tracks them all.
func connectTestHosts(t *testing.T, client *Host, servers ...*Host) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, server := range servers {
		info := peer.AddrInfo{ID: server.PeerID(), Addrs: server.Addrs()}
		if err := client.LibP2PHost().Connect(ctx, info); err != nil {
			t.Fatal(err)
		}
	}
	for client.ConnectedPeers() < len(servers) {
		if ctx.Err() != nil {
			t.Fatal("peers never registered as connected")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The syncer keeps one excluded set for a whole pass, so every peer that fails
// here must be added to the caller's map, not to a copy, or a stalled peer is
// asked again for every batch.
func TestFetchBlocksByRangeWithRetryGrowsCallersExcludedSet(t *testing.T) {
	refuse := func(h *Host) {
		h.RegisterReqRespHandlers(
			func() *StatusMessage { return &StatusMessage{} },
			func([32]byte) *types.SignedBlock { return nil },
			func() uint64 { return 1 },
			func(uint64, uint64) ([]*types.SignedBlock, bool) { return nil, false },
		)
	}
	client, a, b := newTestHost(t, nil), newTestHost(t, nil), newTestHost(t, nil)
	refuse(a)
	refuse(b)
	connectTestHosts(t, client, a, b)

	excluded := map[peer.ID]bool{}
	if _, _, err := client.FetchBlocksByRangeWithRetry(context.Background(), excluded, 1, 1); err == nil {
		t.Fatal("refusing peers served the range")
	}
	if !excluded[a.PeerID()] || !excluded[b.PeerID()] || len(excluded) != 2 {
		t.Fatalf("excluded after the retry = %v, want both refusing peers", excluded)
	}
}
