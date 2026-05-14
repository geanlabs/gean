package p2p

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/types"
)

// Protocol IDs rs.
const (
	StatusProtocol        = "/leanconsensus/req/status/1/ssz_snappy"
	BlocksByRootProtocol  = "/leanconsensus/req/blocks_by_root/1/ssz_snappy"
	BlocksByRangeProtocol = "/leanconsensus/req/blocks_by_range/1/ssz_snappy"
)

// Request/response timeout.
const ReqRespTimeout = 15 * time.Second

// StatusMessage is exchanged on peer connection.
// SSZ wire format: finalized.root (32) | finalized.slot (8) | head.root (32) | head.slot (8) = 80 bytes.
type StatusMessage struct {
	FinalizedRoot [32]byte
	FinalizedSlot uint64
	HeadRoot      [32]byte
	HeadSlot      uint64
}

// MarshalSSZ encodes StatusMessage to SSZ (80 bytes).
// SSZ wire format: finalized (root + slot) then head (root + slot) = 80 bytes.
// where Checkpoint SSZ is { root: H256, slot: u64 }.
func (s *StatusMessage) MarshalSSZ() []byte {
	buf := make([]byte, 80)
	copy(buf[0:32], s.FinalizedRoot[:])
	putUint64LE(buf[32:40], s.FinalizedSlot)
	copy(buf[40:72], s.HeadRoot[:])
	putUint64LE(buf[72:80], s.HeadSlot)
	return buf
}

// UnmarshalSSZ decodes StatusMessage from SSZ.
func (s *StatusMessage) UnmarshalSSZ(buf []byte) error {
	if len(buf) < 80 {
		return fmt.Errorf("status message too short: %d, need 80", len(buf))
	}
	copy(s.FinalizedRoot[:], buf[0:32])
	s.FinalizedSlot = getUint64LE(buf[32:40])
	copy(s.HeadRoot[:], buf[40:72])
	s.HeadSlot = getUint64LE(buf[72:80])
	return nil
}

func putUint64LE(buf []byte, v uint64) {
	for i := 0; i < 8; i++ {
		buf[i] = byte(v >> (i * 8))
	}
}

func getUint64LE(buf []byte) uint64 {
	var v uint64
	for i := 0; i < 8; i++ {
		v |= uint64(buf[i]) << (i * 8)
	}
	return v
}

// RegisterReqRespHandlers registers stream handlers for status, blocks_by_root,
// and blocks_by_range.
//
// currentSlotFn returns the local node's notion of the current slot (used by
// BlocksByRange to enforce the MIN_SLOTS_FOR_BLOCK_REQUESTS sliding-window
// history bound).
//
// blocksInRangeFn returns canonical-chain blocks whose slots fall in
// [startSlot, startSlot+count) in ascending-slot order. Empty slots
// produce no entry.
func (h *Host) RegisterReqRespHandlers(
	statusFn func() *StatusMessage,
	blockByRootFn func(root [32]byte) *types.SignedBlock,
	currentSlotFn func() uint64,
	blocksInRangeFn func(startSlot, count uint64) []*types.SignedBlock,
) {
	// Status handler.
	h.host.SetStreamHandler(protocol.ID(StatusProtocol), func(s network.Stream) {
		defer s.Close()
		handleStatusRequest(s, statusFn)
	})

	// BlocksByRoot handler.
	h.host.SetStreamHandler(protocol.ID(BlocksByRootProtocol), func(s network.Stream) {
		defer s.Close()
		handleBlocksByRootRequest(s, blockByRootFn)
	})

	// BlocksByRange handler.
	h.host.SetStreamHandler(protocol.ID(BlocksByRangeProtocol), func(s network.Stream) {
		defer s.Close()
		handleBlocksByRangeRequest(s, currentSlotFn, blocksInRangeFn)
	})
}

func handleStatusRequest(s network.Stream, statusFn func() *StatusMessage) {
	// Read request payload.
	reqBuf, err := io.ReadAll(io.LimitReader(s, int64(MaxCompressedPayloadSize)))
	if err != nil {
		logger.Warn(logger.Network, "status: read request failed: %v", err)
		return
	}

	// Spec requires a well-formed Status payload (80 bytes after snappy
	// decompression). Reject anything else with INVALID_REQUEST rather than
	// echoing SUCCESS, so misbehaving peers see a clean error.
	if len(reqBuf) == 0 {
		s.Write(EncodeResponse(RespInvalidRequest, []byte("empty status request")))
		return
	}

	payload, err := DecodeReqRespPayload(reqBuf)
	if err != nil {
		logger.Warn(logger.Network, "status: decode request failed: %v", err)
		s.Write(EncodeResponse(RespInvalidRequest, []byte("decode failed")))
		return
	}

	peerStatus := &StatusMessage{}
	if err := peerStatus.UnmarshalSSZ(payload); err != nil {
		logger.Warn(logger.Network, "status: ssz unmarshal failed: %v", err)
		s.Write(EncodeResponse(RespInvalidRequest, []byte("ssz unmarshal failed")))
		return
	}

	logger.Info(logger.Network, "status: peer at slot %d finalized=%d", peerStatus.HeadSlot, peerStatus.FinalizedSlot)

	// Send our status.
	status := statusFn()
	resp := EncodeResponse(RespSuccess, status.MarshalSSZ())
	s.Write(resp)
}

func handleBlocksByRootRequest(s network.Stream, blockByRootFn func(root [32]byte) *types.SignedBlock) {
	reqBuf, err := io.ReadAll(io.LimitReader(s, int64(MaxCompressedPayloadSize)))
	if err != nil {
		logger.Warn(logger.Network, "blocks_by_root: read request failed: %v", err)
		return
	}

	payload, err := DecodeReqRespPayload(reqBuf)
	if err != nil {
		logger.Error(logger.Network, "blocks_by_root: decode request failed: %v", err)
		return
	}

	roots, err := DecodeBlocksByRootRequest(payload)
	if err != nil {
		logger.Error(logger.Network, "blocks_by_root: %v", err)
		return
	}

	// Per spec the root count must be in (0, MAX_REQUEST_BLOCKS]; anything else
	// is INVALID_REQUEST. Mirrors the count check in handleBlocksByRangeRequest.
	if len(roots) == 0 || len(roots) > int(types.MaxRequestBlocks) {
		s.Write(EncodeResponse(RespInvalidRequest, []byte("invalid root count")))
		return
	}

	logger.Info(logger.Network, "blocks_by_root: peer requested %d roots", len(roots))

	for _, root := range roots {
		block := blockByRootFn(root)
		if block == nil {
			// Spec allows silently skipping missing blocks. Log so we can
			// distinguish "we don't have it" from "we have it but the write
			// failed" when triaging known-block test failures from logs.
			logger.Info(logger.Network, "blocks_by_root: block not found root=0x%x", root)
			continue
		}

		blockData, err := block.MarshalSSZ()
		if err != nil {
			logger.Warn(logger.Network, "blocks_by_root: marshal failed root=0x%x: %v", root, err)
			s.Write(EncodeResponse(RespServerError, []byte("marshal failed")))
			continue
		}

		s.Write(EncodeResponse(RespSuccess, blockData))
		logger.Info(logger.Network, "blocks_by_root: served block slot=%d root=0x%x", block.Block.Slot, root)
	}
}

// handleBlocksByRangeRequest processes a BlocksByRange request stream.
//
// Wire flow:
//  1. Read & SSZ-decode BlocksByRangeRequest{ start_slot, count }.
//  2. If start_slot is below the sliding-window history bound
//     (current_slot - MIN_SLOTS_FOR_BLOCK_REQUESTS), reply
//     RESOURCE_UNAVAILABLE.
//  3. Cap count at MAX_REQUEST_BLOCKS to bound work per request.
//  4. Look up canonical-chain blocks via blocksInRangeFn (empty slots are
//     skipped, no chunk emitted for them) and stream one SUCCESS chunk per
//     block in ascending-slot order.
//
// Spec: leanSpec/src/lean_spec/subspecs/networking/reqresp/handler.py.
func handleBlocksByRangeRequest(
	s network.Stream,
	currentSlotFn func() uint64,
	blocksInRangeFn func(startSlot, count uint64) []*types.SignedBlock,
) {
	reqBuf, err := io.ReadAll(io.LimitReader(s, int64(MaxCompressedPayloadSize)))
	if err != nil {
		logger.Warn(logger.Network, "blocks_by_range: read request failed: %v", err)
		return
	}

	payload, err := DecodeReqRespPayload(reqBuf)
	if err != nil {
		logger.Error(logger.Network, "blocks_by_range: decode request failed: %v", err)
		s.Write(EncodeResponse(RespInvalidRequest, []byte("decode failed")))
		return
	}

	req := &types.BlocksByRangeRequest{}
	if err := req.UnmarshalSSZ(payload); err != nil {
		logger.Error(logger.Network, "blocks_by_range: ssz unmarshal failed: %v", err)
		s.Write(EncodeResponse(RespInvalidRequest, []byte("ssz unmarshal failed")))
		return
	}

	// Per leanSpec networking/reqresp/handler.py:285-287, count must be in
	// (0, MAX_REQUEST_BLOCKS]; anything else is INVALID_REQUEST. We reject
	// rather than silently cap so misbehaving peers see a clean error.
	if req.Count == 0 || req.Count > types.MaxRequestBlocks {
		s.Write(EncodeResponse(RespInvalidRequest, []byte("invalid count")))
		return
	}

	currentSlot := currentSlotFn()
	historyFloor := uint64(0)
	if currentSlot > types.MinSlotsForBlockRequests {
		historyFloor = currentSlot - types.MinSlotsForBlockRequests
	}
	if req.StartSlot < historyFloor {
		s.Write(EncodeResponse(RespResourceUnavailable, []byte("start_slot below history horizon")))
		return
	}

	logger.Info(logger.Network, "blocks_by_range: peer requested start_slot=%d count=%d current_slot=%d",
		req.StartSlot, req.Count, currentSlot)

	blocks := blocksInRangeFn(req.StartSlot, req.Count)
	for _, block := range blocks {
		blockData, err := block.MarshalSSZ()
		if err != nil {
			logger.Warn(logger.Network, "blocks_by_range: marshal block at slot %d failed: %v", block.Block.Slot, err)
			s.Write(EncodeResponse(RespServerError, []byte("marshal failed")))
			continue
		}
		s.Write(EncodeResponse(RespSuccess, blockData))
		logger.Info(logger.Network, "blocks_by_range: served slot=%d", block.Block.Slot)
	}
}

// SendStatusRequest sends a status request to a peer and returns their status.
func (h *Host) SendStatusRequest(ctx context.Context, peerID peer.ID, ourStatus *StatusMessage) (*StatusMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, ReqRespTimeout)
	defer cancel()

	s, err := h.host.NewStream(ctx, peerID, protocol.ID(StatusProtocol))
	if err != nil {
		return nil, fmt.Errorf("open status stream: %w", err)
	}
	defer s.Close()

	// Send our status as the request payload.
	reqPayload := EncodeReqRespPayload(ourStatus.MarshalSSZ())
	if _, err := s.Write(reqPayload); err != nil {
		return nil, fmt.Errorf("write status request: %w", err)
	}
	s.CloseWrite()

	// Read response.
	code, respData, err := DecodeResponse(s)
	if err != nil {
		return nil, fmt.Errorf("read status response: %w", err)
	}
	if code != RespSuccess {
		return nil, fmt.Errorf("status response error: code=%d", code)
	}

	peerStatus := &StatusMessage{}
	if err := peerStatus.UnmarshalSSZ(respData); err != nil {
		return nil, fmt.Errorf("unmarshal status: %w", err)
	}
	return peerStatus, nil
}

// FetchBlocksByRoot requests blocks from a peer by their roots.
// Returns successfully decoded blocks (partial success allowed).
func (h *Host) FetchBlocksByRoot(ctx context.Context, peerID peer.ID, roots [][32]byte) ([]*types.SignedBlock, error) {
	ctx, cancel := context.WithTimeout(ctx, ReqRespTimeout)
	defer cancel()

	s, err := h.host.NewStream(ctx, peerID, protocol.ID(BlocksByRootProtocol))
	if err != nil {
		return nil, fmt.Errorf("open blocks_by_root stream: %w", err)
	}
	defer s.Close()

	// Encode as SSZ container: BlocksByRootRequest { roots: List[Root, 1024] }.
	reqPayload := EncodeReqRespPayload(EncodeBlocksByRootRequest(roots))
	if _, err := s.Write(reqPayload); err != nil {
		return nil, fmt.Errorf("write blocks request: %w", err)
	}
	s.CloseWrite()

	// Read multi-chunk response.
	var blocks []*types.SignedBlock
	respBuf, err := io.ReadAll(io.LimitReader(s, int64(MaxCompressedPayloadSize)*int64(len(roots))))
	if err != nil {
		return nil, fmt.Errorf("read blocks response: %w", err)
	}

	reader := bytes.NewReader(respBuf)
	for reader.Len() > 0 {
		code, blockData, err := DecodeResponse(reader)
		if err != nil {
			break // end of stream
		}
		if code != RespSuccess || blockData == nil {
			continue // skip errors, partial success
		}
		block := &types.SignedBlock{}
		if err := block.UnmarshalSSZ(blockData); err != nil {
			continue
		}
		blocks = append(blocks, block)
	}

	return blocks, nil
}

// EncodeBlocksByRootRequest encodes roots as SSZ container: BlocksByRootRequest { roots: List[Root, 1024] }.
// SSZ container with one variable-length field: 4-byte offset + concatenated roots.
// Matches leanSpec networking/reqresp/message.py BlocksByRootRequest.
func EncodeBlocksByRootRequest(roots [][32]byte) []byte {
	rootsData := make([]byte, len(roots)*32)
	for i, root := range roots {
		copy(rootsData[i*32:], root[:])
	}
	// SSZ container: fixed part is 4-byte offset pointing past fixed section.
	container := make([]byte, 4+len(rootsData))
	binary.LittleEndian.PutUint32(container[:4], 4)
	copy(container[4:], rootsData)
	return container
}

// FetchBlocksByRange requests canonical-chain blocks from a peer covering
// [startSlot, startSlot+count). Returns blocks in ascending-slot order with
// slot monotonicity and parent-root continuity validated across empty slots.
//
// Spec: leanSpec/src/lean_spec/subspecs/networking/client/reqresp_client.py.
func (h *Host) FetchBlocksByRange(
	ctx context.Context,
	peerID peer.ID,
	startSlot, count uint64,
) ([]*types.SignedBlock, error) {
	// Input validation.
	if count == 0 {
		return nil, fmt.Errorf("blocks_by_range: count must be > 0")
	}
	if count > types.MaxRequestBlocks {
		return nil, fmt.Errorf("blocks_by_range: count %d > MaxRequestBlocks %d", count, types.MaxRequestBlocks)
	}
	if startSlot > math.MaxUint64-count {
		return nil, fmt.Errorf("blocks_by_range: startSlot+count overflows uint64")
	}
	endSlot := startSlot + count // exclusive

	ctx, cancel := context.WithTimeout(ctx, ReqRespTimeout)
	defer cancel()

	s, err := h.host.NewStream(ctx, peerID, protocol.ID(BlocksByRangeProtocol))
	if err != nil {
		return nil, fmt.Errorf("open blocks_by_range stream: %w", err)
	}
	defer s.Close()

	// SSZ-encode and send the request.
	req := &types.BlocksByRangeRequest{StartSlot: startSlot, Count: count}
	reqSSZ, err := req.MarshalSSZ()
	if err != nil {
		return nil, fmt.Errorf("marshal blocks_by_range request: %w", err)
	}
	if _, err := s.Write(EncodeReqRespPayload(reqSSZ)); err != nil {
		return nil, fmt.Errorf("write blocks_by_range request: %w", err)
	}
	s.CloseWrite()

	// Read multi-chunk response, validate as we go.
	respBuf, err := io.ReadAll(io.LimitReader(s, int64(MaxCompressedPayloadSize)*int64(count)))
	if err != nil {
		return nil, fmt.Errorf("read blocks_by_range response: %w", err)
	}

	var blocks []*types.SignedBlock
	var prevHeaderRoot [32]byte
	var hasPrev bool

	reader := bytes.NewReader(respBuf)
	for reader.Len() > 0 {
		code, blockData, err := DecodeResponse(reader)
		if err != nil {
			break // end of stream
		}
		if code == RespResourceUnavailable {
			return nil, fmt.Errorf("blocks_by_range: peer returned RESOURCE_UNAVAILABLE")
		}
		if code != RespSuccess {
			return nil, fmt.Errorf("blocks_by_range: peer returned error code %d", code)
		}

		block := &types.SignedBlock{}
		if err := block.UnmarshalSSZ(blockData); err != nil {
			return nil, fmt.Errorf("unmarshal block: %w", err)
		}

		// Slot range check.
		if block.Block.Slot < startSlot || block.Block.Slot >= endSlot {
			return nil, fmt.Errorf("blocks_by_range: block slot %d outside requested range [%d, %d)",
				block.Block.Slot, startSlot, endSlot)
		}

		if hasPrev {
			prevSlot := blocks[len(blocks)-1].Block.Slot
			// Slot monotonicity: chunks must be strictly increasing.
			if block.Block.Slot <= prevSlot {
				return nil, fmt.Errorf("blocks_by_range: non-monotonic slots %d <= %d",
					block.Block.Slot, prevSlot)
			}
			// Parent-root continuity across empty slots: this block's parent
			// must be the previous delivered block's header root, regardless
			// of how many empty slots sit between them.
			if block.Block.ParentRoot != prevHeaderRoot {
				return nil, fmt.Errorf("blocks_by_range: parent_root mismatch at slot %d (got 0x%x, want 0x%x)",
					block.Block.Slot, block.Block.ParentRoot, prevHeaderRoot)
			}
		}

		// Compute this block's header root for the next chunk's continuity check.
		bodyRoot, err := block.Block.Body.HashTreeRoot()
		if err != nil {
			return nil, fmt.Errorf("body hash_tree_root: %w", err)
		}
		header := &types.BlockHeader{
			Slot:          block.Block.Slot,
			ProposerIndex: block.Block.ProposerIndex,
			ParentRoot:    block.Block.ParentRoot,
			StateRoot:     block.Block.StateRoot,
			BodyRoot:      bodyRoot,
		}
		prevHeaderRoot, err = header.HashTreeRoot()
		if err != nil {
			return nil, fmt.Errorf("header hash_tree_root: %w", err)
		}
		hasPrev = true

		blocks = append(blocks, block)
	}

	return blocks, nil
}

// DecodeBlocksByRootRequest decodes an SSZ container: BlocksByRootRequest { roots: List[Root, 1024] }.
// Returns the extracted roots.
func DecodeBlocksByRootRequest(payload []byte) ([][32]byte, error) {
	if len(payload) < 4 {
		return nil, fmt.Errorf("payload too short: %d bytes", len(payload))
	}

	offset := binary.LittleEndian.Uint32(payload[:4])
	if offset != 4 {
		return nil, fmt.Errorf("unexpected SSZ offset %d, expected 4", offset)
	}

	rootsData := payload[4:]
	if len(rootsData)%32 != 0 {
		return nil, fmt.Errorf("roots data size %d not multiple of 32", len(rootsData))
	}

	numRoots := len(rootsData) / 32
	roots := make([][32]byte, numRoots)
	for i := 0; i < numRoots; i++ {
		copy(roots[i][:], rootsData[i*32:(i+1)*32])
	}
	return roots, nil
}
