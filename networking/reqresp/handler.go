// Package reqresp implements request/response protocols for the Lean protocol.
package reqresp

import "github.com/devylongs/gean/types"

// Protocol IDs for request/response messages (per devnet0 spec)
const (
	StatusProtocolV1       = "/leanconsensus/req/status/1/"
	BlocksByRootProtocolV1 = "/leanconsensus/req/blocks_by_root/1/"
	MaxRequestBlocks       = 1024 // 2^10
)

// BlockReader provides read access to the block store.
// Satisfied by forkchoice.Store without modification.
type BlockReader interface {
	GetHead() types.Root
	GetBlock(root types.Root) (*types.Block, bool)
	GetLatestFinalized() types.Checkpoint
}

// Handler handles request/response protocol messages.
type Handler struct {
	store BlockReader
}

// NewHandler creates a new request/response handler.
func NewHandler(store BlockReader) *Handler {
	return &Handler{store: store}
}

func (h *Handler) GetStatus() *Status {
	headRoot := h.store.GetHead()
	headBlock, _ := h.store.GetBlock(headRoot)
	return &Status{
		Finalized: h.store.GetLatestFinalized(),
		Head: types.Checkpoint{
			Root: headRoot,
			Slot: headBlock.Slot,
		},
	}
}

func (h *Handler) HandleBlocksByRoot(request *BlocksByRootRequest) []*types.SignedBlock {
	var blocks []*types.SignedBlock

	for _, root := range request.Roots {
		if len(blocks) >= MaxRequestBlocks {
			break
		}

		if block, exists := h.store.GetBlock(root); exists {
			signedBlock := &types.SignedBlock{
				Message:   *block,
				Signature: types.Root{}, // Empty signature for Devnet 0
			}
			blocks = append(blocks, signedBlock)
		}
	}

	return blocks
}

func (h *Handler) ValidatePeerStatus(peerStatus *Status) error {
	if peerStatus.Finalized.Slot > 0 {
		if block, exists := h.store.GetBlock(peerStatus.Finalized.Root); exists {
			if block.Slot != peerStatus.Finalized.Slot {
				return ErrInvalidStatus
			}
		}
	}
	return nil
}
