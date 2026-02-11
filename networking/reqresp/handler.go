// handler.go contains protocol IDs and server-side req/resp request handlers.
// Package reqresp implements request/response protocols (Status, BlocksByRoot).
package reqresp

import "github.com/devylongs/gean/types"

const (
	StatusProtocolV1       = "/leanconsensus/req/status/1/"
	BlocksByRootProtocolV1 = "/leanconsensus/req/blocks_by_root/1/"
	MaxRequestBlocks       = 1024
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

// GetStatus returns the node's current status for the handshake protocol.
func (h *Handler) GetStatus() *Status {
	headRoot := h.store.GetHead()
	var headSlot types.Slot
	if headBlock, exists := h.store.GetBlock(headRoot); exists {
		headSlot = headBlock.Slot
	}
	return &Status{
		Finalized: h.store.GetLatestFinalized(),
		Head: types.Checkpoint{
			Root: headRoot,
			Slot: headSlot,
		},
	}
}

// HandleBlocksByRoot responds to a BlocksByRoot request with matching blocks.
func (h *Handler) HandleBlocksByRoot(request *BlocksByRootRequest) []*types.SignedBlockWithAttestation {
	var blocks []*types.SignedBlockWithAttestation

	for _, root := range request.Roots {
		if len(blocks) >= MaxRequestBlocks {
			break
		}

		if block, exists := h.store.GetBlock(root); exists {
			// Wrap block in envelope. ProposerAttestation and Signatures are empty —
			// the req/resp layer serves raw blocks; full signatures are only in gossip.
			signedBlock := &types.SignedBlockWithAttestation{
				Message: types.BlockWithAttestation{
					Block: *block,
				},
			}
			blocks = append(blocks, signedBlock)
		}
	}

	return blocks
}

// ValidatePeerStatus checks that a peer's status is consistent with our block store.
// If we have the peer's finalized block, its slot must match the claimed finalized slot.
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
