package execution

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/rpc"

	"github.com/geanlabs/gean/internal/types"
)

// fakeEngine is a JSON-RPC server that records requests and replies from a
// script keyed by method. It verifies the bearer token on every request.
type fakeEngine struct {
	t       *testing.T
	secret  JWTSecret
	mu      sync.Mutex
	replies map[string]string
	seen    []rpcRequest
}

type rpcRequest struct {
	ID     uint64 `json:"id"`
	Method string `json:"method"`
	Params []any  `json:"params"`
}

func newFakeEngine(t *testing.T, secret JWTSecret) (*fakeEngine, *httptest.Server) {
	f := &fakeEngine{t: t, secret: secret, replies: map[string]string{}}
	return f, httptest.NewServer(f)
}

func (f *fakeEngine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.checkToken(r.Header.Get("Authorization"))
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		f.t.Errorf("decode request: %v", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.seen = append(f.seen, req)
	reply, ok := f.replies[req.Method]
	f.mu.Unlock()
	if !ok {
		reply = `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}`
	}
	var envelope map[string]any
	if json.Unmarshal([]byte(reply), &envelope) == nil {
		envelope["id"] = req.ID
		encoded, _ := json.Marshal(envelope)
		reply = string(encoded)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(reply))
}

func (f *fakeEngine) checkToken(header string) {
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		f.t.Errorf("missing bearer token")
		return
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		f.t.Errorf("token has %d parts", len(parts))
		return
	}
	claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var c struct {
		Iat int64 `json:"iat"`
	}
	if err := json.Unmarshal(claims, &c); err != nil || c.Iat == 0 {
		f.t.Errorf("token claims %q lack iat", claims)
	}
	if token != f.secret.Token(time.Unix(c.Iat, 0)) {
		f.t.Error("incorrect bearer token")
	}
	if skew := time.Since(time.Unix(c.Iat, 0)); skew > time.Minute || skew < -time.Minute {
		f.t.Errorf("token iat skew %v", skew)
	}
}

func (f *fakeEngine) last() rpcRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seen[len(f.seen)-1]
}

func testClient(t *testing.T) (*Client, *fakeEngine) {
	secret, _ := ParseJWTSecret(testSecretHex)
	fake, server := newFakeEngine(t, secret)
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL, secret)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.rpc.Close)
	return client, fake
}

func TestForkchoiceUpdatedWire(t *testing.T) {
	client, fake := testClient(t)
	fake.replies["engine_forkchoiceUpdatedV3"] = `{"jsonrpc":"2.0","id":1,"result":{"payloadStatus":{"status":"VALID","latestValidHash":"0xabababababababababababababababababababababababababababababababab","validationError":null},"payloadId":"0x0102030405060708"}}`

	state := ForkchoiceState{HeadBlockHash: Hash{1}, SafeBlockHash: Hash{2}, FinalizedBlockHash: Hash{3}}
	attrs := &PayloadAttributes{Timestamp: 4, Withdrawals: []Withdrawal{}, ParentBeaconBlockRoot: Hash{5}}
	result, err := client.ForkchoiceUpdated(t.Context(), state, attrs)
	if err != nil {
		t.Fatal(err)
	}
	if result.PayloadStatus.LatestValidHash == nil || result.PayloadStatus.LatestValidHash[0] != 0xab || result.PayloadID == nil || *result.PayloadID != (PayloadID{1, 2, 3, 4, 5, 6, 7, 8}) {
		t.Fatalf("payload id: %+v", result.PayloadID)
	}
	req := fake.last()
	if req.Method != "engine_forkchoiceUpdatedV3" || len(req.Params) != 2 {
		t.Fatalf("request: %+v", req)
	}
	encoded, _ := json.Marshal(req.Params)
	if !strings.Contains(string(encoded), `"headBlockHash":"0x01`) || !strings.Contains(string(encoded), `"timestamp":"0x4"`) {
		t.Fatalf("params: %s", encoded)
	}

	// Without attributes the second param is null, not omitted.
	fake.replies["engine_forkchoiceUpdatedV3"] = `{"jsonrpc":"2.0","id":1,"result":{"payloadStatus":{"status":"SYNCING"},"payloadId":null}}`
	if result, err := client.ForkchoiceUpdated(t.Context(), state, nil); err != nil || result.PayloadID != nil || result.PayloadStatus.Status != StatusSyncing {
		t.Fatalf("syncing response: %+v, %v", result, err)
	}
	if req := fake.last(); len(req.Params) != 2 || req.Params[1] != nil {
		t.Fatalf("attributes must be an explicit null: %+v", req.Params)
	}
}

func TestGetPayloadUnwrapsEnvelope(t *testing.T) {
	client, fake := testClient(t)
	fake.replies["engine_getPayloadV3"] = `{"jsonrpc":"2.0","id":1,"result":{"executionPayload":{"parentHash":"0x` + strings.Repeat("11", 32) + `","feeRecipient":"0x` + strings.Repeat("22", 20) + `","stateRoot":"0x` + strings.Repeat("33", 32) + `","receiptsRoot":"0x` + strings.Repeat("44", 32) + `","logsBloom":"0x` + strings.Repeat("00", 256) + `","prevRandao":"0x` + strings.Repeat("00", 32) + `","blockNumber":"0x1","gasLimit":"0x1c9c380","gasUsed":"0x0","timestamp":"0x6553f104","extraData":"0x","baseFeePerGas":"0x3b9aca00","blockHash":"0x` + strings.Repeat("55", 32) + `","transactions":[],"withdrawals":[],"blobGasUsed":"0x0","excessBlobGas":"0x0"},"blockValue":"0x0","blobsBundle":{"commitments":[],"proofs":[],"blobs":[]},"shouldOverrideBuilder":false}}`

	payload, err := client.GetPayload(t.Context(), PayloadID{9})
	if err != nil {
		t.Fatal(err)
	}
	if payload.BlockNumber != 1 || payload.BlockHash != [32]byte(Hash{0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55}) {
		t.Fatalf("payload: %+v", payload)
	}
	if payload.Timestamp != 1_700_000_004 || payload.GasLimit != 30_000_000 {
		t.Fatalf("quantities: %+v", payload)
	}
	if req := fake.last(); req.Params[0] != "0x0900000000000000" {
		t.Fatalf("payload id param: %v", req.Params[0])
	}
}

func TestNewPayloadWire(t *testing.T) {
	client, fake := testClient(t)
	fake.replies["engine_newPayloadV3"] = `{"jsonrpc":"2.0","id":1,"result":{"status":"INVALID_BLOCK_HASH","latestValidHash":null,"validationError":"bad hash"}}`

	status, err := client.NewPayload(t.Context(), &types.ExecutionPayload{BlockHash: [32]byte{7}}, [32]byte{8})
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusInvalidBlockHash || status.ValidationError == nil || *status.ValidationError != "bad hash" {
		t.Fatalf("status: %+v", status)
	}
	req := fake.last()
	if len(req.Params) != 3 {
		t.Fatalf("newPayloadV3 takes three params: %+v", req.Params)
	}
	encoded, _ := json.Marshal(req.Params)
	if !strings.Contains(string(encoded), `[],"0x08`) {
		t.Fatalf("expected empty blob hashes then the beacon root: %s", encoded)
	}
}

func TestGenesisBlockHashAndCapabilities(t *testing.T) {
	client, fake := testClient(t)
	fake.replies["eth_getBlockByNumber"] = `{"jsonrpc":"2.0","id":1,"result":{"number":"0x0","hash":"0x` + strings.Repeat("ab", 32) + `","extra":"ignored"}}`
	fake.replies["engine_exchangeCapabilities"] = `{"jsonrpc":"2.0","id":1,"result":["engine_forkchoiceUpdatedV3","engine_newPayloadV3"]}`

	hash, err := client.GenesisBlockHash(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if hash[0] != 0xab || hash[31] != 0xab {
		t.Fatalf("hash: %x", hash)
	}
	if req := fake.last(); req.Params[0] != "0x0" || req.Params[1] != false {
		t.Fatalf("genesis query params: %+v", req.Params)
	}

	supported, err := client.ExchangeCapabilities(t.Context(), Capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if len(supported) != 2 {
		t.Fatalf("supported: %v", supported)
	}
}

func TestErrorsAreTyped(t *testing.T) {
	client, fake := testClient(t)
	fake.replies["engine_getPayloadV3"] = `{"jsonrpc":"2.0","id":1,"error":{"code":-38001,"message":"Unknown payload"}}`
	_, err := client.GetPayload(t.Context(), PayloadID{})
	var rpcErr rpc.Error
	if !errors.As(err, &rpcErr) || rpcErr.ErrorCode() != -38001 || IsTransport(err) {
		t.Fatalf("expected a typed rpc error, got %v", err)
	}

	for _, reply := range []string{
		`{"jsonrpc":"2.0","id":1,"result":null}`,
		`{"jsonrpc":"2.0","id":1,"result":{"status":1}}`,
		`{"jsonrpc":`,
	} {
		fake.replies["engine_newPayloadV3"] = reply
		if _, err := client.NewPayload(t.Context(), &types.ExecutionPayload{}, [32]byte{}); !IsTransport(err) {
			t.Fatalf("unusable reply %s should be a transport error, got %v", reply, err)
		}
	}
	unreachable, err := NewClient("http://127.0.0.1:1", JWTSecret{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unreachable.rpc.Close)
	if _, err := unreachable.GenesisBlockHash(t.Context()); !IsTransport(err) {
		t.Fatalf("connection refused should be a transport error, got %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = client.GenesisBlockHash(ctx)
	if !IsTransport(err) {
		t.Fatalf("cancelled context should be a transport error, got %v", err)
	}
}

func TestRPCRejectsHTTPFailuresAndOversizedReplies(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		t.Run(map[bool]string{false: "HTTP error", true: "response limit"}[oversized], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !oversized {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				// A valid result beyond the cap must fail before it can be decoded.
				io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":"`)
				io.WriteString(w, strings.Repeat("a", maxResponseBytes))
				io.WriteString(w, `"}`)
			}))
			t.Cleanup(server.Close)
			client, err := NewClient(server.URL, JWTSecret{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.rpc.Close)
			var result string
			err = client.rpc.call(t.Context(), "test", nil, &result)
			if !IsTransport(err) || oversized && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("expected transport failure (truncation for oversized replies), got %v", err)
			}
		})
	}
}
