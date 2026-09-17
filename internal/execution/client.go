package execution

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"

	"github.com/geanlabs/gean/internal/types"
)

// DefaultTimeout bounds a single engine call when the caller's context has no
// deadline of its own. Slot-phase callers set tighter ones.
const DefaultTimeout = 8 * time.Second

// Client is the Engine implementation over an execution client's
// authenticated JSON-RPC port.
type Client struct {
	rpc *rpcClient
}

var _ Engine = (*Client)(nil)

// NewClient targets the authenticated engine endpoint, for example
// http://127.0.0.1:8551.
func NewClient(endpoint string, secret JWTSecret) (*Client, error) {
	rpc, err := newRPCClient(endpoint, secret, DefaultTimeout)
	if err != nil {
		return nil, err
	}
	return &Client{rpc: rpc}, nil
}

func (c *Client) ForkchoiceUpdated(ctx context.Context, state ForkchoiceState, attrs *PayloadAttributes) (ForkchoiceUpdatedResult, error) {
	var result ForkchoiceUpdatedResult
	if err := c.rpc.call(ctx, "engine_forkchoiceUpdatedV3", []any{state, attrs}, &result); err != nil {
		return ForkchoiceUpdatedResult{}, err
	}
	return result, nil
}

// GetPayload keeps only the payload from the reply envelope; the block value,
// blobs bundle, and builder override flag have no consensus meaning here.
func (c *Client) GetPayload(ctx context.Context, id PayloadID) (*types.ExecutionPayload, error) {
	var envelope engine.ExecutionPayloadEnvelope
	if err := c.rpc.call(ctx, "engine_getPayloadV3", []any{id}, &envelope); err != nil {
		return nil, err
	}
	payload, err := FromExecutableData(envelope.ExecutionPayload)
	if err != nil {
		return nil, &TransportError{Method: "engine_getPayloadV3", Err: err}
	}
	if err := payload.ValidateExecutionFeatures(); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *Client) NewPayload(ctx context.Context, payload *types.ExecutionPayload, parentBeaconBlockRoot [32]byte) (PayloadStatus, error) {
	if err := payload.ValidateExecutionFeatures(); err != nil {
		return PayloadStatus{}, err
	}
	var status PayloadStatus
	// The feature check above guarantees there are no blob versioned hashes.
	params := []any{ToExecutableData(payload), []common.Hash{}, common.Hash(parentBeaconBlockRoot)}
	if err := c.rpc.call(ctx, "engine_newPayloadV3", params, &status); err != nil {
		return PayloadStatus{}, err
	}
	return status, nil
}

// blockHashOnly reads just the hash of an eth_getBlockByNumber reply.
type blockHashOnly struct {
	Hash common.Hash `json:"hash"`
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
