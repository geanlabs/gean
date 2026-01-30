package types

// Status is the handshake message exchanged upon connection.
// It allows nodes to verify compatibility and determine sync status.
type Status struct {
	Finalized Checkpoint
	Head      Checkpoint
}

// BlocksByRootRequest is a request for blocks by their root hashes.
type BlocksByRootRequest struct {
	Roots []Root `ssz-max:"1024" ssz-size:"?,32"`
}
