package execution

import (
	"context"
	"fmt"
	"time"

	"github.com/geanlabs/gean/internal/types"
)

// DefaultTimeout bounds a single engine call when the caller's context has no
// deadline of its own. Slot-phase callers set tighter ones.
const DefaultTimeout = 8 * time.Second

// Client is the Engine implementation over a real execution client.
type Client struct {
	rpc *rpcClient
}

var _ Engine = (*Client)(nil)

// NewClient targets the authenticated engine endpoint, for example
// http://127.0.0.1:8551.
func NewClient(endpoint string, secret JWTSecret) *Client {
	return &Client{rpc: newRPCClient(endpoint, secret, DefaultTimeout)}
}

func (c *Client) ForkchoiceUpdated(ctx context.Context, state ForkchoiceState, attrs *PayloadAttributes) (ForkchoiceUpdatedResult, error) {
	var result ForkchoiceUpdatedResult
	params := []any{state, nil}
	if attrs != nil {
		params[1] = attrs
	}
	if err := c.rpc.call(ctx, "engine_forkchoiceUpdatedV3", params, &result); err != nil {
		return ForkchoiceUpdatedResult{}, err
	}
	return result, nil
}

// getPayloadEnvelope is the engine_getPayloadV3 reply. Only the payload is
// used; the block value, blobs bundle, and builder override flag are dropped.
type getPayloadEnvelope struct {
	ExecutionPayload *Payload `json:"executionPayload"`
}

func (c *Client) GetPayload(ctx context.Context, id PayloadID) (*types.ExecutionPayload, error) {
	var envelope getPayloadEnvelope
	if err := c.rpc.call(ctx, "engine_getPayloadV3", []any{id}, &envelope); err != nil {
		return nil, err
	}
	if envelope.ExecutionPayload == nil {
		return nil, &TransportError{Method: "engine_getPayloadV3", Err: fmt.Errorf("reply carries no executionPayload")}
	}
	return PayloadFromWire(envelope.ExecutionPayload), nil
}

func (c *Client) NewPayload(ctx context.Context, payload *types.ExecutionPayload, parentBeaconBlockRoot [32]byte) (PayloadStatus, error) {
	var status PayloadStatus
	// No blob transactions, so the expected versioned hashes are empty.
	params := []any{PayloadToWire(payload), []Hash{}, Hash(parentBeaconBlockRoot)}
	if err := c.rpc.call(ctx, "engine_newPayloadV3", params, &status); err != nil {
		return PayloadStatus{}, err
	}
	return status, nil
}

// blockHashOnly reads just the hash of an eth_getBlockByNumber reply.
type blockHashOnly struct {
	Hash Hash `json:"hash"`
}

func (c *Client) GenesisBlockHash(ctx context.Context) ([32]byte, error) {
	var block blockHashOnly
	if err := c.rpc.call(ctx, "eth_getBlockByNumber", []any{"0x0", false}, &block); err != nil {
		return [32]byte{}, err
	}
	return block.Hash, nil
}

func (c *Client) ExchangeCapabilities(ctx context.Context, offered []string) ([]string, error) {
	var supported []string
	if err := c.rpc.call(ctx, "engine_exchangeCapabilities", []any{offered}, &supported); err != nil {
		return nil, err
	}
	return supported, nil
}
