// Package execution speaks the Engine API to an execution client over its
// authenticated JSON-RPC port. It carries no consensus logic: the node decides
// when to call and what a verdict means.
package execution

import (
	"context"

	"github.com/geanlabs/gean/internal/types"
)

// Capabilities are the engine methods gean calls, advertised in the startup
// handshake. They are the Cancun set. Gean enforces empty withdrawals and no
// blob transactions; execution requests and Prague methods are not supported.
var Capabilities = []string{
	"engine_forkchoiceUpdatedV3",
	"engine_getPayloadV3",
	"engine_newPayloadV3",
}

// Engine is implemented by Client and the test Mock.
type Engine interface {
	// With attrs, ForkchoiceUpdated also starts a payload build.
	ForkchoiceUpdated(ctx context.Context, state ForkchoiceState, attrs *PayloadAttributes) (ForkchoiceUpdatedResult, error)
	GetPayload(ctx context.Context, id PayloadID) (*types.ExecutionPayload, error)
	// parentBeaconBlockRoot must be the consensus parent root of the carrying block.
	NewPayload(ctx context.Context, payload *types.ExecutionPayload, parentBeaconBlockRoot [32]byte) (PayloadStatus, error)
	GenesisBlockHash(ctx context.Context) ([32]byte, error)
	ExchangeCapabilities(ctx context.Context, offered []string) ([]string, error)
}
