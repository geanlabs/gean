package p2p

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/core/p2p/reqresp"
	"github.com/geanlabs/gean/internal/lean/metrics"
	"github.com/geanlabs/gean/internal/lean/types"
)

func handleBlocksByRangeRequest(
	stream network.Stream,
	currentSlotFn func() uint64,
	blocksInRangeFn func(startSlot, count uint64) ([]*types.SignedBlock, bool),
) {
	reqresp.ArmReadDeadline(stream)
	reqBuf, err := io.ReadAll(io.LimitReader(stream, int64(reqresp.MaxCompressedPayloadSize)))
	if err != nil {
		logger.Warn(logger.Network, "blocks_by_range: read request failed: %v", err)
		return
	}

	payload, err := reqresp.DecodePayload(reqBuf)
	if err != nil {
		logger.Error(logger.Network, "blocks_by_range: decode request failed: %v", err)
		reqresp.WriteResponse(stream, "blocks_by_range", reqresp.RespInvalidRequest, []byte("decode failed"))
		return
	}

	req := &types.BlocksByRangeRequest{}
	if err := req.UnmarshalSSZ(payload); err != nil {
		logger.Error(logger.Network, "blocks_by_range: ssz unmarshal failed: %v", err)
		reqresp.WriteResponse(stream, "blocks_by_range", reqresp.RespInvalidRequest, []byte("ssz unmarshal failed"))
		return
	}
	if err := validateRangeRequest(req.StartSlot, req.Count); err != nil {
		reqresp.WriteResponse(stream, "blocks_by_range", reqresp.RespInvalidRequest, []byte("invalid count"))
		return
	}

	currentSlot := currentSlotFn()
	historyFloor := uint64(0)
	if currentSlot > types.MinSlotsForBlockRequests {
		historyFloor = currentSlot - types.MinSlotsForBlockRequests
	}
	if req.StartSlot < historyFloor {
		reqresp.WriteResponse(stream, "blocks_by_range", reqresp.RespResourceUnavailable, []byte("start_slot below history horizon"))
		return
	}

	logger.Info(logger.Network, "blocks_by_range: peer requested start_slot=%d count=%d current_slot=%d",
		req.StartSlot, req.Count, currentSlot)

	blocks, complete := blocksInRangeFn(req.StartSlot, req.Count)
	// An incomplete ancestor chain is not an empty range. Serving it as a clean
	// empty stream tells the requester this range does not exist, so it stops
	// instead of asking a peer that can answer.
	if !complete {
		reqresp.WriteResponse(stream, "blocks_by_range", reqresp.RespResourceUnavailable, []byte("history below stored chain"))
		return
	}

	for _, block := range blocks {
		if block == nil || block.Block == nil {
			continue
		}
		blockData, err := block.MarshalSSZ()
		if err != nil {
			logger.Warn(logger.Network, "blocks_by_range: marshal block at slot %d failed: %v", block.Block.Slot, err)
			if !reqresp.WriteResponse(stream, "blocks_by_range", reqresp.RespServerError, []byte("marshal failed")) {
				return
			}
			continue
		}
		if !reqresp.WriteResponse(stream, "blocks_by_range", reqresp.RespSuccess, blockData) {
			return
		}
		logger.Info(logger.Network, "blocks_by_range: served slot=%d", block.Block.Slot)
	}
}

func (h *Host) FetchBlocksByRange(
	ctx context.Context,
	peerID peer.ID,
	startSlot, count uint64,
) (blocks []*types.SignedBlock, err error) {
	if err := validateRangeRequest(startSlot, count); err != nil {
		return nil, err
	}
	endSlot := startSlot + count
	defer func(parent context.Context) { h.RecordRequest(parent, peerID, err) }(ctx)

	ctx, cancel := context.WithTimeout(ctx, reqresp.Timeout)
	defer cancel()

	stream, err := h.NewStream(ctx, peerID, BlocksByRangeProtocol)
	if err != nil {
		return nil, fmt.Errorf("open blocks_by_range stream: %w", err)
	}
	defer stream.Close()

	req := &types.BlocksByRangeRequest{StartSlot: startSlot, Count: count}
	reqSSZ, err := req.MarshalSSZ()
	if err != nil {
		return nil, fmt.Errorf("marshal blocks_by_range request: %w", err)
	}
	reqresp.ArmWriteDeadline(stream)
	reqBytes := reqresp.EncodePayload(reqSSZ)
	metrics.ObserveReqRespRequestSize("blocks_by_range", len(reqBytes))
	if _, err := stream.Write(reqBytes); err != nil {
		return nil, fmt.Errorf("write blocks_by_range request: %w", err)
	}
	stream.CloseWrite()

	var prevHeaderRoot [32]byte
	var hasPrev bool

	reader := bufio.NewReader(io.LimitReader(stream, int64(reqresp.MaxCompressedPayloadSize)*int64(count)))
	for {
		code, blockData, err := reqresp.ReadChunk(stream, reader, "blocks_by_range", MaxSignedBlockSize)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode blocks_by_range response: %w", err)
		}
		if code == reqresp.RespResourceUnavailable {
			return nil, fmt.Errorf("blocks_by_range: peer returned RESOURCE_UNAVAILABLE")
		}
		if code != reqresp.RespSuccess {
			return nil, fmt.Errorf("blocks_by_range: peer returned error code %d", code)
		}

		block := &types.SignedBlock{}
		if err := block.UnmarshalSSZ(blockData); err != nil {
			return nil, fmt.Errorf("unmarshal block: %w", err)
		}
		if block.Block == nil || block.Block.Body == nil {
			return nil, fmt.Errorf("blocks_by_range: malformed block")
		}
		if block.Block.Slot < startSlot || block.Block.Slot >= endSlot {
			return nil, fmt.Errorf("blocks_by_range: block slot %d outside requested range [%d, %d)",
				block.Block.Slot, startSlot, endSlot)
		}

		if hasPrev {
			prevSlot := blocks[len(blocks)-1].Block.Slot
			if block.Block.Slot <= prevSlot {
				return nil, fmt.Errorf("blocks_by_range: non-monotonic slots %d <= %d",
					block.Block.Slot, prevSlot)
			}
			if block.Block.ParentRoot != prevHeaderRoot {
				return nil, fmt.Errorf("blocks_by_range: parent_root mismatch at slot %d (got 0x%x, want 0x%x)",
					block.Block.Slot, block.Block.ParentRoot, prevHeaderRoot)
			}
		}

		header, err := block.Block.Header()
		if err != nil {
			return nil, fmt.Errorf("block header: %w", err)
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

func validateRangeRequest(startSlot, count uint64) error {
	if count == 0 {
		return fmt.Errorf("blocks_by_range: count must be > 0")
	}
	if count > types.MaxRequestBlocks {
		return fmt.Errorf("blocks_by_range: count %d > MaxRequestBlocks %d", count, types.MaxRequestBlocks)
	}
	if startSlot > math.MaxUint64-count {
		return fmt.Errorf("blocks_by_range: startSlot+count overflows uint64")
	}
	return nil
}
