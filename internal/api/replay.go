package api

import (
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// Replay routes serve raw SSZ so an offline tool can re-run, or prove, the
// state transition of a block the node imported.
//
//	GET /lean/v0/blocks/{block_id}/ssz  SignedBlock
//	GET /lean/v0/states/{state_id}      post-state of that block
//
// An id is head, justified, finalized, a decimal slot on the canonical chain,
// or a 0x-prefixed block root. States older than the finalized block are pruned
// on finalization and answer 404.

var errUnknownID = errors.New("unknown block id")

// resolveBlockID maps an id to a block root known to the store.
func resolveBlockID(s *store.ConsensusStore, id string) ([32]byte, error) {
	switch id {
	case "head":
		return nonZero(s.Head())
	case "justified":
		return nonZero(s.LatestJustified().Root)
	case "finalized":
		return nonZero(s.LatestFinalized().Root)
	}
	if strings.HasPrefix(id, "0x") || strings.HasPrefix(id, "0X") {
		b, err := hex.DecodeString(id[2:])
		if err != nil || len(b) != 32 {
			return [32]byte{}, errUnknownID
		}
		return [32]byte(b), nil
	}
	slot, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return [32]byte{}, errUnknownID
	}
	return canonicalRootAtSlot(s, slot)
}

// canonicalRootAtSlot walks back from head. A slot with no block on the
// canonical chain, or below the node's anchor, has no root.
func canonicalRootAtSlot(s *store.ConsensusStore, slot uint64) ([32]byte, error) {
	root := s.Head()
	for {
		header := s.GetBlockHeader(root)
		if header == nil || header.Slot < slot {
			return [32]byte{}, errUnknownID
		}
		if header.Slot == slot {
			return root, nil
		}
		if header.Slot == 0 {
			return [32]byte{}, errUnknownID
		}
		root = header.ParentRoot
	}
}

func nonZero(root [32]byte) ([32]byte, error) {
	if root == types.ZeroRoot {
		return root, errUnknownID
	}
	return root, nil
}

func BlockSSZHandler(s *store.ConsensusStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		root, err := resolveBlockID(s, r.PathValue("block_id"))
		if err != nil {
			http.Error(w, "block not found", http.StatusNotFound)
			return
		}
		signedBlock := s.GetSignedBlock(root)
		if signedBlock == nil {
			http.Error(w, "block not found", http.StatusNotFound)
			return
		}
		data, err := signedBlock.MarshalSSZ()
		if err != nil {
			http.Error(w, "ssz marshal failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(data)
	}
}

// StateSSZHandler serves the post-state of a block with the cached
// latest-header state root cleared, as the transition leaves it. Its hash tree
// root then equals the block's StateRoot, which is also the next block's
// pre-state root, so proofs over consecutive blocks chain.
func StateSSZHandler(s *store.ConsensusStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		root, err := resolveBlockID(s, r.PathValue("state_id"))
		if err != nil {
			http.Error(w, "state not found", http.StatusNotFound)
			return
		}
		state := s.GetState(root)
		if state == nil || state.LatestBlockHeader == nil {
			http.Error(w, "state not found or pruned", http.StatusNotFound)
			return
		}
		state.LatestBlockHeader.StateRoot = types.ZeroRoot
		data, err := state.MarshalSSZ()
		if err != nil {
			http.Error(w, "ssz marshal failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(data)
	}
}
