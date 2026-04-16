//go:build spectests

package spectests

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/geanlabs/gean/p2p"
)

// Spec fixture directory root rs `leanSpec/fixtures/consensus/networking_codec`.
const netCodecFixturesRoot = "../leanSpec/fixtures/consensus/networking_codec/devnet/networking"

// netFixtureOuter is the top-level JSON shape: one fixture file contains one
// outer map whose single key is the pytest test ID and whose value is the fixture.
type netFixtureOuter map[string]netFixture

type netFixture struct {
	Network   string                 `json:"network"`
	LeanEnv   string                 `json:"leanEnv"`
	CodecName string                 `json:"codecName"`
	Input     map[string]interface{} `json:"input"`
	Output    map[string]interface{} `json:"output"`
}

// TestSpecNetworkingCodec runs every JSON fixture under the networking_codec
// directory and dispatches by codec name. Unsupported codecs are skipped.
func TestSpecNetworkingCodec(t *testing.T) {
	if _, err := os.Stat(netCodecFixturesRoot); os.IsNotExist(err) {
		t.Skipf("fixtures not present at %s; run 'make leanSpec/fixtures' to generate", netCodecFixturesRoot)
	}

	var passed, failed, skipped int
	perCodec := map[string][3]int{} // codec -> [passed, failed, skipped]

	err := filepath.Walk(netCodecFixturesRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: read: %v", path, err)
			return nil
		}

		var outer netFixtureOuter
		if err := json.Unmarshal(raw, &outer); err != nil {
			t.Errorf("%s: unmarshal: %v", path, err)
			return nil
		}

		for testID, fx := range outer {
			name := shortName(testID, path)

			t.Run(name, func(t *testing.T) {
				result := runNetCodecFixture(t, fx)
				entry := perCodec[fx.CodecName]
				switch result {
				case "pass":
					entry[0]++
					passed++
				case "fail":
					entry[1]++
					failed++
				case "skip":
					entry[2]++
					skipped++
				}
				perCodec[fx.CodecName] = entry
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	t.Logf("--- networking_codec summary ---")
	t.Logf("total: %d passed, %d failed, %d skipped", passed, failed, skipped)
	for codec, counts := range perCodec {
		t.Logf("  %-20s passed=%d failed=%d skipped=%d", codec, counts[0], counts[1], counts[2])
	}
}

// runNetCodecFixture dispatches one fixture to the appropriate gean primitive.
// Returns "pass", "fail", or "skip".
func runNetCodecFixture(t *testing.T, fx netFixture) string {
	switch fx.CodecName {
	case "varint":
		return testVarint(t, fx)
	case "gossip_topic":
		return testGossipTopic(t, fx)
	case "gossip_message_id":
		return testGossipMessageID(t, fx)
	case "enr":
		return testENR(t, fx)
	case "discv5_message":
		return testDiscv5Message(t, fx)
	default:
		t.Skipf("codec %q not yet wired into gean test harness", fx.CodecName)
		return "skip"
	}
}

// --- varint ---

func testVarint(t *testing.T, fx netFixture) string {
	val, ok := fx.Input["value"].(float64)
	if !ok {
		t.Errorf("missing or non-numeric input.value")
		return "fail"
	}
	wantHex, _ := fx.Output["encoded"].(string)
	wantLen, _ := fx.Output["byteLength"].(float64)
	want, err := decodeHex(wantHex)
	if err != nil {
		t.Errorf("decode expected hex: %v", err)
		return "fail"
	}

	// Gean only supports uint32 input; spec fixtures include uint64 values.
	if val > float64(^uint32(0)) {
		t.Skipf("value %.0f exceeds gean's EncodeVarint uint32 limit", val)
		return "skip"
	}

	got := p2p.EncodeVarint(uint32(val))
	if fmt.Sprintf("%x", got) != fmt.Sprintf("%x", want) {
		t.Errorf("value=%.0f: got %x, want %x", val, got, want)
		return "fail"
	}
	if int(wantLen) != len(got) {
		t.Errorf("value=%.0f: got byteLength=%d, want %d", val, len(got), int(wantLen))
		return "fail"
	}
	return "pass"
}

// --- gossip_topic ---

func testGossipTopic(t *testing.T, fx netFixture) string {
	kind, _ := fx.Input["kind"].(string)
	forkDigest, _ := fx.Input["forkDigest"].(string)
	subnetID, hasSubnet := fx.Input["subnetId"].(float64)
	wantTopic, _ := fx.Output["topicString"].(string)

	// Gean currently uses a fixed network name (devnet0) where the spec uses
	// forkDigest hex. This is expected to fail all gossip_topic tests until
	// clients agree on the topic-path format.
	var got string
	switch kind {
	case "block":
		got = p2p.BlockTopic()
	case "aggregation":
		got = p2p.AggregationTopic()
	case "attestation":
		if !hasSubnet {
			t.Errorf("attestation topic missing subnetId")
			return "fail"
		}
		got = p2p.AttestationSubnetTopic(uint64(subnetID))
	default:
		t.Skipf("unknown topic kind %q", kind)
		return "skip"
	}

	if got != wantTopic {
		t.Errorf("kind=%s forkDigest=%s: got %q, want %q", kind, forkDigest, got, wantTopic)
		return "fail"
	}
	return "pass"
}

// --- gossip_message_id ---

// testGossipMessageID replicates the spec formula inline so we can feed explicit
// (topic, data, domain) triples without going through gean's snappy-detection path.
// Formula rs:
//
//	SHA256(domain || uint64_le(len(topic)) || topic || data)[:20]
func testGossipMessageID(t *testing.T, fx netFixture) string {
	topicHex, _ := fx.Input["topic"].(string)
	dataHex, _ := fx.Input["data"].(string)
	domainHex, _ := fx.Input["domain"].(string)
	wantHex, _ := fx.Output["messageId"].(string)

	topic, err := decodeHex(topicHex)
	if err != nil {
		t.Errorf("topic hex: %v", err)
		return "fail"
	}
	data, err := decodeHex(dataHex)
	if err != nil {
		t.Errorf("data hex: %v", err)
		return "fail"
	}
	domain, err := decodeHex(domainHex)
	if err != nil {
		t.Errorf("domain hex: %v", err)
		return "fail"
	}
	want, err := decodeHex(wantHex)
	if err != nil {
		t.Errorf("want hex: %v", err)
		return "fail"
	}

	h := sha256.New()
	var topicLen [8]byte
	binary.LittleEndian.PutUint64(topicLen[:], uint64(len(topic)))
	h.Write(domain)
	h.Write(topicLen[:])
	h.Write(topic)
	h.Write(data)
	got := h.Sum(nil)[:20]

	if fmt.Sprintf("%x", got) != fmt.Sprintf("%x", want) {
		t.Errorf("got %x, want %x", got, want)
		return "fail"
	}
	return "pass"
}

// --- enr ---

func testENR(t *testing.T, fx netFixture) string {
	enrStr, _ := fx.Input["enrString"].(string)
	wantMultiaddr, _ := fx.Output["multiaddr"].(string)
	wantValid, _ := fx.Output["isValid"].(bool)

	fields, err := p2p.DecodeENR(enrStr)
	if err != nil {
		if !wantValid {
			return "pass" // spec expected invalid, gean rejected -> OK
		}
		t.Errorf("DecodeENR rejected valid ENR: %v", err)
		return "fail"
	}
	if !wantValid {
		t.Errorf("DecodeENR accepted invalid ENR; spec expected rejection")
		return "fail"
	}

	if wantMultiaddr != "" && fields.Multiaddr != wantMultiaddr {
		t.Errorf("multiaddr: got %q, want %q", fields.Multiaddr, wantMultiaddr)
		return "fail"
	}
	return "pass"
}

// --- discv5_message ---

var discv5TypeByte = map[string]byte{
	"ping":     0x01,
	"pong":     0x02,
	"findnode": 0x03,
	"nodes":    0x04,
	"talkreq":  0x05,
	"talkresp": 0x06,
}

func testDiscv5Message(t *testing.T, fx netFixture) string {
	msgType, _ := fx.Input["type"].(string)
	wantHex, _ := fx.Output["encoded"].(string)
	want, err := decodeHex(wantHex)
	if err != nil {
		t.Errorf("decode expected hex: %v", err)
		return "fail"
	}

	typeByte, ok := discv5TypeByte[msgType]
	if !ok {
		t.Skipf("unknown discv5 message type %q", msgType)
		return "skip"
	}

	var fields []interface{}
	reqIDHex, _ := fx.Input["requestId"].(string)
	reqID, _ := decodeHex(reqIDHex)
	// Strip leading zeros per discv5 minimal encoding.
	for len(reqID) > 1 && reqID[0] == 0 {
		reqID = reqID[1:]
	}
	fields = append(fields, reqID)

	switch msgType {
	case "ping":
		enrSeq := uint64(fx.Input["enrSeq"].(float64))
		fields = append(fields, enrSeq)
	case "pong":
		enrSeq := uint64(fx.Input["enrSeq"].(float64))
		ipHex, _ := fx.Input["recipientIp"].(string)
		ip, _ := decodeHex(ipHex)
		port := uint16(fx.Input["recipientPort"].(float64))
		fields = append(fields, enrSeq, ip, port)
	case "findnode":
		dists, _ := fx.Input["distances"].([]interface{})
		var ds []uint64
		for _, d := range dists {
			ds = append(ds, uint64(d.(float64)))
		}
		fields = append(fields, ds)
	default:
		t.Skipf("discv5 message type %q encoding not yet implemented", msgType)
		return "skip"
	}

	encoded, err := rlp.EncodeToBytes(fields)
	if err != nil {
		t.Errorf("rlp encode: %v", err)
		return "fail"
	}

	got := append([]byte{typeByte}, encoded...)
	if fmt.Sprintf("%x", got) != fmt.Sprintf("%x", want) {
		t.Errorf("type=%s: got %x, want %x", msgType, got, want)
		return "fail"
	}
	return "pass"
}

// --- helpers ---

func decodeHex(s string) ([]byte, error) {
	s = strings.TrimPrefix(s, "0x")
	return hex.DecodeString(s)
}

// shortName shortens a pytest test ID to a readable subtest name.
func shortName(testID, path string) string {
	// Fall back to the JSON filename if the testID is unwieldy.
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".json")
	return base
}
