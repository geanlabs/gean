// Package execution speaks the Engine API to an execution client over its
// authenticated JSON-RPC port. It carries no consensus logic: the node decides
// when to call and what a verdict means.
package execution

import (
	"context"

	"github.com/geanlabs/gean/internal/types"
)

// Capabilities are the engine methods gean calls, advertised in the startup
// handshake. They are the Cancun set: gean carries no blob transactions and no
// execution-layer requests, so the Prague methods, which need both to round
// trip through the block, are not used.
var Capabilities = []string{
	"engine_forkchoiceUpdatedV3",
	"engine_getPayloadV3",
	"engine_newPayloadV3",
}

// Engine is what the node depends on. Client implements it against a real
// execution client; Mock implements it for tests.
type Engine interface {
	// ForkchoiceUpdated tells the execution client where the chain's head,
	// safe, and finalized blocks are. With attributes it also starts building
	// the next payload and returns the id to fetch it by.
	ForkchoiceUpdated(ctx context.Context, state ForkchoiceState, attrs *PayloadAttributes) (ForkchoiceUpdatedResult, error)
	// GetPayload fetches the payload built under a previously returned id.
	GetPayload(ctx context.Context, id PayloadID) (*types.ExecutionPayload, error)
	// NewPayload asks the execution client to execute and validate a payload.
	// parentBeaconBlockRoot is part of the execution block hash, so every
	// node must pass the same value: the consensus parent root of the block
	// carrying the payload.
	NewPayload(ctx context.Context, payload *types.ExecutionPayload, parentBeaconBlockRoot [32]byte) (PayloadStatus, error)
	// GenesisBlockHash reads the execution client's block 0 hash, which the
	// network config must match.
	GenesisBlockHash(ctx context.Context) ([32]byte, error)
	// ExchangeCapabilities returns the engine methods the execution client
	// supports out of the ones offered.
	ExchangeCapabilities(ctx context.Context, offered []string) ([]string, error)
}
