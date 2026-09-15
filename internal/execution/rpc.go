package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
)

const maxResponseBytes = 64 << 20

// TransportError means no usable verdict arrived; the request may have executed.
type TransportError struct {
	Method string
	Err    error
}

func (e *TransportError) Error() string { return fmt.Sprintf("%s: transport: %v", e.Method, e.Err) }
func (e *TransportError) Unwrap() error { return e.Err }

func IsTransport(err error) bool {
	var transport *TransportError
	return errors.As(err, &transport)
}

type rpcClient struct{ *rpc.Client }

func newRPCClient(endpoint string, secret JWTSecret, timeout time.Duration) (*rpcClient, error) {
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		return nil, fmt.Errorf("execution endpoint must use HTTP or HTTPS")
	}
	c, err := rpc.DialOptions(context.Background(), endpoint,
		rpc.WithHTTPClient(&http.Client{Timeout: timeout, Transport: limitedTransport{http.DefaultTransport}}),
		rpc.WithHTTPAuth(func(h http.Header) error {
			h.Set("Authorization", "Bearer "+secret.Token(time.Now()))
			return nil
		}))
	return &rpcClient{c}, err
}

// Bound responses before geth's JSON decoder allocates the payload.
type limitedTransport struct{ http.RoundTripper }

func (t limitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.RoundTripper.RoundTrip(req)
	if err == nil {
		resp.Body = struct {
			io.Reader
			io.Closer
		}{io.LimitReader(resp.Body, maxResponseBytes), resp.Body}
	}
	return resp, err
}

// Preserve remote RPC errors separately from unreachable or malformed replies.
func (c *rpcClient) call(ctx context.Context, method string, params []any, result any) error {
	var raw json.RawMessage
	if err := c.CallContext(ctx, &raw, method, params...); err != nil {
		var remote rpc.Error
		if errors.As(err, &remote) {
			return fmt.Errorf("%s: %w", method, err)
		}
		return &TransportError{Method: method, Err: err}
	}
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return &TransportError{Method: method, Err: errors.New("empty result")}
	}
	if err := json.Unmarshal(raw, result); err != nil {
		return &TransportError{Method: method, Err: err}
	}
	return nil
}
