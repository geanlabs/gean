//go:build spectests

package lean

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"

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
	case "decode_failure", "gossipsub_rpc", "peer_id", "reqresp_request", "reqresp_response",
		"reqresp_response_stream", "snappy_block", "snappy_frame":
		skip(t, skipNotImplemented, codec.Kind+" codec")
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
	// gean's varint only encodes request/response length prefixes, which fit
	// in 32 bits.
	if codec.Value > math.MaxUint32 {
		skip(t, skipNotImplemented, "varints above 32 bits")
	}
	got := reqresp.EncodeVarint(uint32(codec.Value))
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

	RLP                specfixtures.Unchecked `json:"rlp"`
	NodeID             specfixtures.Unchecked `json:"nodeId"`
	PublicKey          specfixtures.Unchecked `json:"publicKey"`
	IdentityScheme     specfixtures.Unchecked `json:"identityScheme"`
	SignatureValid     specfixtures.Unchecked `json:"signatureValid"`
	IP4                specfixtures.Unchecked `json:"ip4"`
	UDPPort            specfixtures.Unchecked `json:"udpPort"`
	QUICPort           specfixtures.Unchecked `json:"quicPort"`
	IsAggregator       specfixtures.Unchecked `json:"isAggregator"`
	AttestationSubnets specfixtures.Unchecked `json:"attestationSubnets"`
	Eth2Data           specfixtures.Unchecked `json:"eth2Data"`
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
}
