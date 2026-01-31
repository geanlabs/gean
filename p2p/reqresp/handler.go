// Package reqresp implements request/response protocols for the Lean protocol.
package reqresp

import (
	"errors"

	"github.com/devylongs/gean/forkchoice"
	"github.com/devylongs/gean/types"
)

// Protocol IDs for request/response messages (per devnet0 spec)
const (
	StatusProtocolV1       = "/leanconsensus/req/status/1/"
	BlocksByRootProtocolV1 = "/leanconsensus/req/blocks_by_root/1/"
	MaxRequestBlocks       = 1024 // 2^10
)

// Handler handles request/response protocol messages.
type Handler struct {
	store *forkchoice.Store
}

// NewHandler creates a new request/response handler.
func NewHandler(store *forkchoice.Store) *Handler {
	return &Handler{store: store}
}

// GetStatus returns our current status.
func (h *Handler) GetStatus() *types.Status {
	h.store.RLock()
	defer h.store.RUnlock()

	headBlock := h.store.Blocks[h.store.Head]
	return &types.Status{
		Finalized: h.store.LatestFinalized,
		Head: types.Checkpoint{
			Root: h.store.Head,
			Slot: headBlock.Slot,
		},
	}
}

// HandleBlocksByRoot processes a BlocksByRoot request.
// Returns the requested blocks that we have available.
func (h *Handler) HandleBlocksByRoot(request *types.BlocksByRootRequest) []*types.SignedBlock {
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

	return blocks
}

// Errors for req/resp handling
var (
	ErrInvalidStatus = errors.New("invalid peer status")
)

// ValidatePeerStatus validates an incoming peer's status.
// Returns an error if the peer is on a different chain.
func (h *Handler) ValidatePeerStatus(peerStatus *types.Status) error {
	h.store.RLock()
	defer h.store.RUnlock()

	// If we have the peer's finalized block, verify slot matches
	if peerStatus.Finalized.Slot > 0 {
		if block, exists := h.store.Blocks[peerStatus.Finalized.Root]; exists {
			if block.Slot != peerStatus.Finalized.Slot {
				return ErrInvalidStatus
			}
		}
	}

	return nil
}
