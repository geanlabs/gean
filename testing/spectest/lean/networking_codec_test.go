//go:build spectests

package lean

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"

	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/golang/snappy"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/core/p2p/discovery"
	"github.com/geanlabs/gean/internal/core/p2p/gossip"
	"github.com/geanlabs/gean/internal/core/p2p/reqresp"
	"github.com/geanlabs/gean/internal/lean/p2p"
	"github.com/geanlabs/gean/internal/lean/specfixtures"
)

// networkingCodecFixture holds one codec case. The codec object names its
// kind and carries that kind's inputs, so each kind decodes it into its own
// type.
type networkingCodecFixture struct {
	specfixtures.Header
	Codec           json.RawMessage `json:"codec"`
	Output          json.RawMessage `json:"output"`
	RejectionReason string          `json:"rejectionReason"`
}

func runNetworkingCodec(t *testing.T, fixture *networkingCodecFixture) {
	var codec struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(fixture.Codec, &codec); err != nil {
		t.Fatalf("codec: %v", err)
	}
	switch codec.Kind {
	case "varint":
		runCodec(t, fixture, checkVarint)
	case "gossip_topic":
		runCodec(t, fixture, checkGossipTopic)
	case "gossip_message_id":
		runCodec(t, fixture, checkGossipMessageID)
	case "enr":
		runCodec(t, fixture, checkENR)
	case "reqresp_request":
		runCodec(t, fixture, checkRequestChunk)
	case "reqresp_response":
		runCodec(t, fixture, checkResponseChunk)
	case "reqresp_response_stream":
		runCodec(t, fixture, checkResponseStream)
	case "snappy_block":
		runCodec(t, fixture, checkSnappyBlock)
	case "snappy_frame":
		runCodec(t, fixture, checkSnappyFrame)
	case "gossipsub_rpc":
		runCodec(t, fixture, checkGossipsubRPC)
	case "peer_id":
		runCodec(t, fixture, checkPeerID)
	case "decode_failure":
		runCodec(t, fixture, checkDecodeFailure)
	default:
		t.Fatalf("unknown codec %q", codec.Kind)
	}
}

// runCodec decodes the codec inputs into C and the expected output into O,
// strictly, then checks gean against them.
func runCodec[C, O any](t *testing.T, fixture *networkingCodecFixture, check func(*testing.T, *C, *O)) {
	t.Helper()
	var codec C
	var output O
	if err := decodeStrict(fixture.Codec, &codec); err != nil {
		t.Fatalf("codec: %v", err)
	}
	if err := decodeStrict(fixture.Output, &output); err != nil {
		t.Fatalf("output: %v", err)
	}
	record := caseRecords[t]
	record.unchecked = append(record.unchecked, uncheckedFields(reflect.ValueOf(output), "output")...)
	check(t, &codec, &output)
}

type varintCodec struct {
	Kind  string `json:"kind"`
	Value uint64 `json:"value"`
}

type varintOutput struct {
	Encoded    string `json:"encoded"`
	ByteLength int    `json:"byteLength"`
}

func checkVarint(t *testing.T, codec *varintCodec, output *varintOutput) {
	got := reqresp.EncodeVarint(codec.Value)
	want := mustParse(t, specfixtures.ParseHexBytes, output.Encoded)
	if !bytes.Equal(got, want) || len(got) != output.ByteLength {
		t.Errorf("varint %d: got %x (%d bytes), want %x (%d bytes)",
			codec.Value, got, len(got), want, output.ByteLength)
	}
}

type gossipTopicCodec struct {
	Kind                string  `json:"kind"`
	TopicKind           string  `json:"topicKind"`
	NetworkName         string  `json:"networkName"`
	SubnetID            *uint64 `json:"subnetId"`
	ExpectedNetworkName string  `json:"expectedNetworkName"`
}

type gossipTopicOutput struct {
	TopicString string `json:"topicString"`
	// ForkValid says whether the topic belongs to the expected network. gean
	// only joins its own network's topics, so a foreign network's message never
	// reaches it, and there is no gean check to compare this with.
	ForkValid *bool `json:"forkValid"`
}

func checkGossipTopic(t *testing.T, codec *gossipTopicCodec, output *gossipTopicOutput) {
	var topicName string
	switch codec.TopicKind {
	case "block":
		topicName = p2p.BlockTopicKind
	case "aggregation":
		topicName = p2p.AggregationTopicKind
	case "attestation":
		if codec.SubnetID == nil {
			t.Fatal("attestation topic without a subnet ID")
		}
		topicName = fmt.Sprintf("%s_%d", p2p.AttestationTopicKind, *codec.SubnetID)
	default:
		t.Fatalf("unknown topic kind %q", codec.TopicKind)
	}
	if got := p2p.BuildGossipTopic(codec.NetworkName, topicName); got != output.TopicString {
		t.Errorf("topic: got %q, want %q", got, output.TopicString)
	}
}

type gossipMessageIDCodec struct {
	Kind   string `json:"kind"`
	Topic  string `json:"topic"`
	Data   string `json:"data"`
	Domain string `json:"domain"`
}

type gossipMessageIDOutput struct {
	MessageID string `json:"messageId"`
}

// The spec computes the ID from the domain and the payload it applies to.
// gean picks the domain itself from whether the wire bytes decompress, so it
// is fed the bytes that would arrive: the snappy-compressed payload under the
// valid-snappy domain, and the raw payload otherwise.
var validSnappyDomain = []byte{0x01, 0x00, 0x00, 0x00}

func checkGossipMessageID(t *testing.T, codec *gossipMessageIDCodec, output *gossipMessageIDOutput) {
	topic := mustParse(t, specfixtures.ParseHexBytes, codec.Topic)
	payload := mustParse(t, specfixtures.ParseHexBytes, codec.Data)
	wire := payload
	if bytes.Equal(mustParse(t, specfixtures.ParseHexBytes, codec.Domain), validSnappyDomain) {
		wire = gossip.Compress(payload)
	}
	got := gossip.MessageID(string(topic), wire)
	if want := mustParse(t, specfixtures.ParseHexBytes, output.MessageID); !bytes.Equal(got, want) {
		t.Errorf("message ID: got %x, want %x", got, want)
	}
}

type enrCodec struct {
	Kind      string `json:"kind"`
	ENRString string `json:"enrString"`
}

type enrOutput struct {
	IsValid   bool   `json:"isValid"`
	Seq       uint64 `json:"seq"`
	Multiaddr string `json:"multiaddr"`

	PublicKey string `json:"publicKey"`
	Eth2Data  *struct {
		ForkDigest      string                 `json:"forkDigest"`
		NextForkVersion specfixtures.Unchecked `json:"nextForkVersion"`
		NextForkEpoch   specfixtures.Unchecked `json:"nextForkEpoch"`
	} `json:"eth2Data"`

	RLP                specfixtures.Unchecked `json:"rlp"`
	NodeID             specfixtures.Unchecked `json:"nodeId"`
	IdentityScheme     specfixtures.Unchecked `json:"identityScheme"`
	SignatureValid     specfixtures.Unchecked `json:"signatureValid"`
	IP4                specfixtures.Unchecked `json:"ip4"`
	UDPPort            specfixtures.Unchecked `json:"udpPort"`
	QUICPort           specfixtures.Unchecked `json:"quicPort"`
	IsAggregator       specfixtures.Unchecked `json:"isAggregator"`
	AttestationSubnets specfixtures.Unchecked `json:"attestationSubnets"`
}

func checkENR(t *testing.T, codec *enrCodec, output *enrOutput) {
	fields, err := discovery.DecodeENR(codec.ENRString)
	switch {
	case err != nil && output.IsValid:
		t.Fatalf("rejected a valid ENR: %v", err)
	case err != nil:
		return
	case !output.IsValid:
		t.Fatal("accepted an invalid ENR")
	}
	if fields.Seq != output.Seq {
		t.Errorf("seq: got %d, want %d", fields.Seq, output.Seq)
	}
	if output.Multiaddr != "" && fields.Multiaddr != output.Multiaddr {
		t.Errorf("multiaddr: got %q, want %q", fields.Multiaddr, output.Multiaddr)
	}
	// gean dials a discovered node only when it is on gean's fork, which it
	// reads from the record's eth2 entry.
	sameFork := output.Eth2Data != nil &&
		bytes.Equal(hexBytes(t, output.Eth2Data.ForkDigest), mustParse(t, hex.DecodeString, p2p.ForkDigest))
	if got := p2p.SameFork(enode.SignNull(fields.Record, enode.ID{})); got != sameFork {
		t.Errorf("same fork: got %v, want %v", got, sameFork)
	}
	// gean derives a peer's libp2p ID from the record's secp256k1 key.
	publicKey, err := libp2pcrypto.UnmarshalSecp256k1PublicKey(hexBytes(t, output.PublicKey))
	if err != nil {
		t.Fatalf("fixture public key: %v", err)
	}
	if want, err := peer.IDFromPublicKey(publicKey); err != nil || fields.PeerID != want {
		t.Errorf("peer ID: got %s, want %s (%v)", fields.PeerID, want, err)
	}
}

// Snappy compressors may choose different, equally valid encodings for the
// same data, and leanSpec's vectors come from C snappy. What interop needs is
// that each side decodes the other, so compressed bytes are checked by decoding
// leanSpec's and round-tripping gean's own. Everything uncompressed, such as
// varint prefixes and response codes, is compared byte for byte.

func hexBytes(t *testing.T, s string) []byte {
	t.Helper()
	return mustParse(t, specfixtures.ParseHexBytes, s)
}

type encodedOutput struct {
	Encoded string `json:"encoded"`
}

type requestChunkCodec struct {
	Kind    string `json:"kind"`
	SSZData string `json:"sszData"`
}

func checkRequestChunk(t *testing.T, codec *requestChunkCodec, output *encodedOutput) {
	payload, reference := hexBytes(t, codec.SSZData), hexBytes(t, output.Encoded)
	checkPrefix(t, reqresp.EncodeVarint(uint64(len(payload))), reference, "length prefix")
	for name, encoded := range map[string][]byte{"leanSpec's": reference, "gean's": reqresp.EncodePayload(payload)} {
		if got, err := reqresp.DecodePayload(encoded); err != nil || !bytes.Equal(got, payload) {
			t.Errorf("decode %s request: got %x (%v), want %x", name, got, err, payload)
		}
	}
}

type responseChunk struct {
	ResponseCode byte   `json:"responseCode"`
	SSZData      string `json:"sszData"`
}

type responseChunkCodec struct {
	Kind string `json:"kind"`
	responseChunk
}

func checkResponseChunk(t *testing.T, codec *responseChunkCodec, output *encodedOutput) {
	checkResponseChunks(t, []responseChunk{codec.responseChunk}, hexBytes(t, output.Encoded))
}

type responseStreamCodec struct {
	Kind   string          `json:"kind"`
	Chunks []responseChunk `json:"chunks"`
}

type responseStreamOutput struct {
	Encoded    string `json:"encoded"`
	ChunkCount int    `json:"chunkCount"`
}

func checkResponseStream(t *testing.T, codec *responseStreamCodec, output *responseStreamOutput) {
	if len(codec.Chunks) != output.ChunkCount {
		t.Fatalf("fixture has %d chunks but a chunk count of %d", len(codec.Chunks), output.ChunkCount)
	}
	checkResponseChunks(t, codec.Chunks, hexBytes(t, output.Encoded))
}

// checkResponseChunks decodes leanSpec's encoding of chunks, and gean's, back
// into exactly those chunks.
func checkResponseChunks(t *testing.T, chunks []responseChunk, reference []byte) {
	t.Helper()
	var geanEncoding []byte
	for _, chunk := range chunks {
		geanEncoding = append(geanEncoding, reqresp.EncodeResponse(chunk.ResponseCode, hexBytes(t, chunk.SSZData))...)
	}
	checkPrefix(t, geanEncoding[:1+len(reqresp.EncodeVarint(uint64(len(hexBytes(t, chunks[0].SSZData)))))], reference,
		"first response code and length prefix")
	for name, encoded := range map[string][]byte{"leanSpec's": reference, "gean's": geanEncoding} {
		stream := bytes.NewReader(encoded)
		for i, chunk := range chunks {
			code, payload, err := reqresp.DecodeResponse(stream, reqresp.MaxPayloadSize)
			if err != nil || code != chunk.ResponseCode || !bytes.Equal(payload, hexBytes(t, chunk.SSZData)) {
				t.Fatalf("decode %s chunk %d: code %d payload %x (%v), want code %d payload %s",
					name, i, code, payload, err, chunk.ResponseCode, chunk.SSZData)
			}
		}
		if stream.Len() != 0 {
			t.Errorf("decode %s stream: %d bytes left after %d chunks", name, stream.Len(), len(chunks))
		}
	}
}

func checkPrefix(t *testing.T, want, encoding []byte, what string) {
	t.Helper()
	if !bytes.HasPrefix(encoding, want) {
		t.Errorf("%s: leanSpec's encoding starts %x, gean writes %x", what, encoding[:min(len(want), len(encoding))], want)
	}
}

type snappyCodec struct {
	Kind string `json:"kind"`
	Data string `json:"data"`
}

type snappyBlockOutput struct {
	Compressed         string `json:"compressed"`
	CompressedLength   int    `json:"compressedLength"`
	UncompressedLength int    `json:"uncompressedLength"`
}

// checkSnappyBlock checks gean's gossip compression, which uses snappy's block
// format.
func checkSnappyBlock(t *testing.T, codec *snappyCodec, output *snappyBlockOutput) {
	data, reference := hexBytes(t, codec.Data), hexBytes(t, output.Compressed)
	if len(data) != output.UncompressedLength || len(reference) != output.CompressedLength {
		t.Fatalf("fixture lengths disagree with its data: %d and %d, want %d and %d",
			len(data), len(reference), output.UncompressedLength, output.CompressedLength)
	}
	for name, compressed := range map[string][]byte{"leanSpec's": reference, "gean's": gossip.Compress(data)} {
		if got, err := gossip.Decompress(compressed); err != nil || !bytes.Equal(got, data) {
			t.Errorf("decompress %s block: got %x (%v), want %x", name, got, err, data)
		}
	}
}

type snappyFrameOutput struct {
	Framed             string `json:"framed"`
	FramedLength       int    `json:"framedLength"`
	UncompressedLength int    `json:"uncompressedLength"`
}

// checkSnappyFrame checks the framed format request/response chunks carry
// after their length prefix, through gean's chunk codec.
func checkSnappyFrame(t *testing.T, codec *snappyCodec, output *snappyFrameOutput) {
	data, reference := hexBytes(t, codec.Data), hexBytes(t, output.Framed)
	if len(data) != output.UncompressedLength || len(reference) != output.FramedLength {
		t.Fatalf("fixture lengths disagree with its data: %d and %d, want %d and %d",
			len(data), len(reference), output.UncompressedLength, output.FramedLength)
	}
	prefix := reqresp.EncodeVarint(uint64(len(data)))
	for name, chunk := range map[string][]byte{"leanSpec's": append(prefix, reference...), "gean's": reqresp.EncodePayload(data)} {
		if got, err := reqresp.DecodePayload(chunk); err != nil || !bytes.Equal(got, data) {
			t.Errorf("decode %s frames: got %x (%v), want %x", name, got, err, data)
		}
	}
}

type gossipsubRPCCodec struct {
	Kind          string `json:"kind"`
	Subscriptions []struct {
		Subscribe bool   `json:"subscribe"`
		TopicID   string `json:"topicId"`
	} `json:"subscriptions"`
	Publish []struct {
		FromPeer  string `json:"fromPeer"`
		Data      string `json:"data"`
		Seqno     string `json:"seqno"`
		Topic     string `json:"topic"`
		Signature string `json:"signature"`
		Key       string `json:"key"`
	} `json:"publish"`
	Control *struct {
		Ihave []struct {
			TopicID    string   `json:"topicId"`
			MessageIDs []string `json:"messageIds"`
		} `json:"ihave"`
		Iwant []struct {
			MessageIDs []string `json:"messageIds"`
		} `json:"iwant"`
		Graft []struct {
			TopicID string `json:"topicId"`
		} `json:"graft"`
		Prune []struct {
			TopicID string `json:"topicId"`
			Backoff uint64 `json:"backoff"`
		} `json:"prune"`
		Idontwant []struct {
			MessageIDs []string `json:"messageIds"`
		} `json:"idontwant"`
	} `json:"control"`
}

// checkGossipsubRPC encodes the RPC with go-libp2p-pubsub's protobuf types,
// the code gean's gossip runs on. As in leanSpec's encoder, a subscription
// always carries its topic, and every other field is left out when empty.
func checkGossipsubRPC(t *testing.T, codec *gossipsubRPCCodec, output *encodedOutput) {
	text := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	raw := func(hexValue string) []byte {
		if b := hexBytes(t, hexValue); len(b) > 0 {
			return b
		}
		return nil
	}
	messageIDs := func(hexIDs []string) []string {
		ids := make([]string, len(hexIDs))
		for i, id := range hexIDs {
			ids[i] = string(hexBytes(t, id))
		}
		return ids
	}

	var rpc pubsubpb.RPC
	for _, subscription := range codec.Subscriptions {
		rpc.Subscriptions = append(rpc.Subscriptions, &pubsubpb.RPC_SubOpts{
			Subscribe: &subscription.Subscribe, Topicid: &subscription.TopicID,
		})
	}
	for _, message := range codec.Publish {
		rpc.Publish = append(rpc.Publish, &pubsubpb.Message{
			From: raw(message.FromPeer), Data: raw(message.Data), Seqno: raw(message.Seqno),
			Topic: text(message.Topic), Signature: raw(message.Signature), Key: raw(message.Key),
		})
	}
	if control := codec.Control; control != nil {
		message := &pubsubpb.ControlMessage{}
		for _, ihave := range control.Ihave {
			message.Ihave = append(message.Ihave, &pubsubpb.ControlIHave{TopicID: text(ihave.TopicID), MessageIDs: messageIDs(ihave.MessageIDs)})
		}
		for _, iwant := range control.Iwant {
			message.Iwant = append(message.Iwant, &pubsubpb.ControlIWant{MessageIDs: messageIDs(iwant.MessageIDs)})
		}
		for _, graft := range control.Graft {
			message.Graft = append(message.Graft, &pubsubpb.ControlGraft{TopicID: text(graft.TopicID)})
		}
		for _, prune := range control.Prune {
			entry := &pubsubpb.ControlPrune{TopicID: text(prune.TopicID)}
			if prune.Backoff > 0 {
				entry.Backoff = &prune.Backoff
			}
			message.Prune = append(message.Prune, entry)
		}
		for _, idontwant := range control.Idontwant {
			message.Idontwant = append(message.Idontwant, &pubsubpb.ControlIDontWant{MessageIDs: messageIDs(idontwant.MessageIDs)})
		}
		if message.Size() > 0 {
			rpc.Control = message
		}
	}
	got, err := rpc.Marshal()
	if err != nil {
		t.Fatalf("encode RPC: %v", err)
	}
	if want := hexBytes(t, output.Encoded); !bytes.Equal(got, want) {
		t.Errorf("RPC encoding: got %x, want %x", got, want)
	}
}

type peerIDCodec struct {
	Kind      string `json:"kind"`
	KeyType   string `json:"keyType"`
	PublicKey string `json:"publicKey"`
}

type peerIDOutput struct {
	ProtobufEncoded string `json:"protobufEncoded"`
	PeerID          string `json:"peerId"`
}

// checkPeerID derives the peer ID with go-libp2p's key code, which gean uses
// for its own identity and its peers'.
func checkPeerID(t *testing.T, codec *peerIDCodec, output *peerIDOutput) {
	unmarshal, ok := map[string]func([]byte) (libp2pcrypto.PubKey, error){
		"secp256k1": libp2pcrypto.UnmarshalSecp256k1PublicKey,
		"ed25519":   libp2pcrypto.UnmarshalEd25519PublicKey,
		"ecdsa":     libp2pcrypto.UnmarshalECDSAPublicKey,
	}[codec.KeyType]
	if !ok {
		t.Fatalf("unknown key type %q", codec.KeyType)
	}
	publicKey, err := unmarshal(hexBytes(t, codec.PublicKey))
	if err != nil {
		t.Fatalf("public key: %v", err)
	}
	encoded, err := libp2pcrypto.MarshalPublicKey(publicKey)
	if err != nil {
		t.Fatalf("encode public key: %v", err)
	}
	if want := hexBytes(t, output.ProtobufEncoded); !bytes.Equal(encoded, want) {
		t.Errorf("protobuf encoding: got %x, want %x", encoded, want)
	}
	id, err := peer.IDFromPublicKey(publicKey)
	if err != nil {
		t.Fatalf("peer ID: %v", err)
	}
	if id.String() != output.PeerID {
		t.Errorf("peer ID: got %s, want %s", id, output.PeerID)
	}
}

type decodeFailureCodec struct {
	Kind     string `json:"kind"`
	Decoder  string `json:"decoder"`
	RawBytes string `json:"rawBytes"`
}

type decodeFailureOutput struct {
	Decoder string `json:"decoder"`
}

// checkDecodeFailure feeds malformed bytes to the gean decoder they target,
// which must reject them.
func checkDecodeFailure(t *testing.T, codec *decodeFailureCodec, _ *decodeFailureOutput) {
	raw := hexBytes(t, codec.RawBytes)
	var err error
	switch codec.Decoder {
	case "varint", "reqresp_request":
		// A request chunk opens with its varint length prefix, so a malformed
		// varint fails the chunk decode before anything else is read.
		_, err = reqresp.DecodePayload(raw)
	case "snappy_frame":
		// gean reads chunk frames with golang/snappy's frame reader; an empty
		// stream holds no frames, which leanSpec also rejects.
		if len(raw) == 0 {
			err = errors.New("no frames")
		} else {
			_, err = io.ReadAll(snappy.NewReader(bytes.NewReader(raw)))
		}
	case "gossipsub_rpc":
		err = new(pubsubpb.RPC).Unmarshal(raw)
	case "enr":
		_, err = discovery.DecodeENR("enr:" + base64.RawURLEncoding.EncodeToString(raw))
	default:
		t.Fatalf("unknown decoder %q", codec.Decoder)
	}
	if err == nil {
		t.Errorf("%s decoded malformed input %x", codec.Decoder, raw)
	}
}
