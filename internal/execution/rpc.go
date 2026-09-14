package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// maxResponseBytes bounds a single engine response. A built payload is at
// most a few megabytes of transactions; anything larger is not a payload.
const maxResponseBytes = 64 << 20

// TransportError is a failure to reach the execution client or to read its
// reply: the request may or may not have been processed.
type TransportError struct {
	Method string
	Err    error
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("%s: transport: %v", e.Method, e.Err)
}

func (e *TransportError) Unwrap() error { return e.Err }

// RPCError is a JSON-RPC error envelope the execution client returned.
type RPCError struct {
	Method  string
	Code    int64
	Message string
	Data    json.RawMessage
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("%s: rpc error %d: %s", e.Method, e.Code, e.Message)
}

// IsTransport reports whether the call never got a well-formed answer, which
// is the case a caller may treat as "unknown" rather than as a verdict.
func IsTransport(err error) bool {
	var transport *TransportError
	return errors.As(err, &transport)
}

type rpcClient struct {
	http   *http.Client
	url    string
	secret JWTSecret
	now    func() time.Time
	nextID atomic.Uint64
}

func newRPCClient(url string, secret JWTSecret, timeout time.Duration) *rpcClient {
	return &rpcClient{
		http:   &http.Client{Timeout: timeout},
		url:    url,
		secret: secret,
		now:    time.Now,
	}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      uint64 `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int64           `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	} `json:"error"`
}

// call performs one JSON-RPC request. The context bounds the whole exchange;
// callers set a deadline sized to the slot phase the call serves.
func (c *rpcClient) call(ctx context.Context, method string, params []any, result any) error {
	if params == nil {
		params = []any{}
	}
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: c.nextID.Add(1), Method: method, Params: params})
	if err != nil {
		return fmt.Errorf("%s: encode request: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return &TransportError{Method: method, Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.secret.Token(c.now()))

	resp, err := c.http.Do(req)
	if err != nil {
		return &TransportError{Method: method, Err: err}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return &TransportError{Method: method, Err: err}
	}
	if resp.StatusCode != http.StatusOK {
		return &TransportError{Method: method, Err: fmt.Errorf("http %d: %s", resp.StatusCode, bytes.TrimSpace(raw))}
	}

	var envelope rpcResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return &TransportError{Method: method, Err: fmt.Errorf("decode envelope: %w", err)}
	}
	if envelope.Error != nil {
		return &RPCError{Method: method, Code: envelope.Error.Code, Message: envelope.Error.Message, Data: envelope.Error.Data}
	}
	if result == nil {
		return nil
	}
	if len(envelope.Result) == 0 || bytes.Equal(envelope.Result, []byte("null")) {
		return &TransportError{Method: method, Err: errors.New("empty result")}
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return &TransportError{Method: method, Err: fmt.Errorf("decode result: %w", err)}
	}
	return nil
}
