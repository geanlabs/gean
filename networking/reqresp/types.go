// types.go contains SSZ request/response message types for the req/resp protocols.
package reqresp

import "github.com/devylongs/gean/types"

// Status is the handshake message exchanged upon connection.
type Status struct {
	Finalized types.Checkpoint
	Head      types.Checkpoint
}

// BlocksByRootRequest is a request for blocks by their hash tree roots.
type BlocksByRootRequest struct {
	Roots []types.Root `ssz-max:"1024" ssz-size:"?,32"`
}
