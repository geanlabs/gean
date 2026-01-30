// Package reqresp implements request/response protocols for the Lean protocol.
package reqresp

import (
	"github.com/devylongs/gean/forkchoice"
	"github.com/devylongs/gean/types"
)

// Protocol IDs for request/response messages
const (
	StatusProtocolV1       = "/leanconsensus/req/status/1/ssz_snappy"
	BlocksByRootProtocolV1 = "/leanconsensus/req/blocks_by_root/1/ssz_snappy"
	MaxRequestBlocks       = 1024 // 2^10
)

// BlocksByRootResponse is the response containing requested signed blocks.
// Note: Status and BlocksByRootRequest are defined in types package with SSZ encoding.
type BlocksByRootResponse struct {
	Blocks []*types.SignedBlock
}

// NewStatus creates a Status message from the current store state.
func NewStatus(store *forkchoice.Store) *types.Status {
	store.RLock()
	defer store.RUnlock()

	headBlock := store.Blocks[store.Head]
	return &types.Status{
		Finalized: store.LatestFinalized,
		Head: types.Checkpoint{
			Root: store.Head,
			Slot: headBlock.Slot,
		},
	}
}

// Handler handles request/response protocol messages.
type Handler struct {
	store *forkchoice.Store
}

// NewHandler creates a new request/response handler.
func NewHandler(store *forkchoice.Store) *Handler {
	return &Handler{store: store}
}

// HandleStatus processes an incoming Status request.
// Returns our current status for the handshake.
func (h *Handler) HandleStatus(peerStatus *types.Status) *types.Status {
	return NewStatus(h.store)
}

// HandleBlocksByRoot processes a BlocksByRoot request.
// Returns the requested blocks that we have available.
func (h *Handler) HandleBlocksByRoot(request *types.BlocksByRootRequest) *BlocksByRootResponse {
	h.store.RLock()
	defer h.store.RUnlock()

	var blocks []*types.SignedBlock

	for _, root := range request.Roots {
		if len(blocks) >= MaxRequestBlocks {
			break
		}

		if block, exists := h.store.Blocks[root]; exists {
			// Wrap block in SignedBlock (signature is placeholder in Devnet 0)
			signedBlock := &types.SignedBlock{
				Message:   *block,
				Signature: types.Root{},
			}
			blocks = append(blocks, signedBlock)
		}
	}

	return &BlocksByRootResponse{Blocks: blocks}
}

// ValidatePeerStatus validates an incoming peer's status.
// Returns an error if the peer is on a different chain or too far behind.
func (h *Handler) ValidatePeerStatus(peerStatus *types.Status) error {
	h.store.RLock()
	defer h.store.RUnlock()

	// For Devnet 0, basic validation:
	// - Peer's finalized checkpoint should not conflict with ours
	// - If we have a finalized block at peer's finalized slot, roots should match

	if peerStatus.Finalized.Slot > 0 {
		// Check if we have this slot in our history
		if block, exists := h.store.Blocks[peerStatus.Finalized.Root]; exists {
			if block.Slot != peerStatus.Finalized.Slot {
				return ErrInvalidStatus
			}
		}
	}

	return nil
}

// Errors for req/resp handling
var (
	ErrInvalidStatus = &Error{Message: "invalid peer status"}
)

// Error represents a request/response protocol error.
type Error struct {
	Message string
}

func (e *Error) Error() string {
	return e.Message
}
