package p2p

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/core/p2p/reqresp"
	"github.com/geanlabs/gean/internal/lean/types"
)

func handleBlocksByRootRequest(stream network.Stream, blockByRootFn func(root [32]byte) *types.SignedBlock) {
	reqresp.ArmReadDeadline(stream)
	reqBuf, err := io.ReadAll(io.LimitReader(stream, int64(reqresp.MaxCompressedPayloadSize)))
	if err != nil {
		logger.Warn(logger.Network, "blocks_by_root: read request failed: %v", err)
		return
	}

	payload, err := reqresp.DecodePayload(reqBuf)
	if err != nil {
		logger.Error(logger.Network, "blocks_by_root: decode request failed: %v", err)
		reqresp.WriteResponse(stream, "blocks_by_root", reqresp.RespInvalidRequest, []byte("decode failed"))
		return
	}

	req := &types.BlocksByRootRequest{}
	if err := req.UnmarshalSSZ(payload); err != nil {
		logger.Error(logger.Network, "blocks_by_root: %v", err)
		reqresp.WriteResponse(stream, "blocks_by_root", reqresp.RespInvalidRequest, []byte("ssz unmarshal failed"))
		return
	}
	roots := req.Roots
	if err := validateRootCount(len(roots)); err != nil {
		reqresp.WriteResponse(stream, "blocks_by_root", reqresp.RespInvalidRequest, []byte("invalid root count"))
		return
	}

	logger.Info(logger.Network, "blocks_by_root: peer requested %d roots", len(roots))

	for _, root := range roots {
		block := blockByRootFn(root)
		if block == nil {
			logger.Info(logger.Network, "blocks_by_root: block not found root=0x%x", root)
			continue
		}

		blockData, err := block.MarshalSSZ()
		if err != nil {
			logger.Warn(logger.Network, "blocks_by_root: marshal failed root=0x%x: %v", root, err)
			if !reqresp.WriteResponse(stream, "blocks_by_root", reqresp.RespServerError, []byte("marshal failed")) {
				return
			}
			continue
		}

		if !reqresp.WriteResponse(stream, "blocks_by_root", reqresp.RespSuccess, blockData) {
			return
		}
		logger.Info(logger.Network, "blocks_by_root: served block slot=%d root=0x%x", block.Block.Slot, root)
	}
}

func (h *Host) FetchBlocksByRoot(ctx context.Context, peerID peer.ID, roots [][32]byte) (blocks []*types.SignedBlock, err error) {
	if err := validateRootCount(len(roots)); err != nil {
		return nil, err
	}
	defer func(parent context.Context) { h.RecordRequest(parent, peerID, err) }(ctx)

	ctx, cancel := context.WithTimeout(ctx, reqresp.Timeout)
	defer cancel()

	stream, err := h.NewStream(ctx, peerID, BlocksByRootProtocol)
	if err != nil {
		return nil, fmt.Errorf("open blocks_by_root stream: %w", err)
	}
	defer stream.Close()

	req, err := (&types.BlocksByRootRequest{Roots: roots}).MarshalSSZ()
	if err != nil {
		return nil, fmt.Errorf("marshal blocks_by_root request: %w", err)
	}
	if err := reqresp.WriteRequest(stream, "blocks_by_root", req); err != nil {
		return nil, fmt.Errorf("write blocks request: %w", err)
	}

	requestedRoots := requestedRootSet(roots)
	seenRoots := make(map[[32]byte]bool, len(roots))
	reader := bufio.NewReader(io.LimitReader(stream, int64(reqresp.MaxCompressedPayloadSize)*int64(len(roots))))
	for {
		code, blockData, err := reqresp.ReadChunk(stream, reader, "blocks_by_root", MaxSignedBlockSize)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode blocks_by_root response: %w", err)
		}
		if code != reqresp.RespSuccess || blockData == nil {
			return nil, fmt.Errorf("blocks_by_root: peer returned error code %d", code)
		}
		block := &types.SignedBlock{}
		if err := block.UnmarshalSSZ(blockData); err != nil {
			return nil, fmt.Errorf("unmarshal block: %w", err)
		}
		if block.Block == nil {
			return nil, fmt.Errorf("blocks_by_root: malformed block")
		}
		if err := validateFetchedBlockRoot(block, requestedRoots, seenRoots); err != nil {
			return nil, fmt.Errorf("blocks_by_root: %w", err)
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func requestedRootSet(roots [][32]byte) map[[32]byte]bool {
	set := make(map[[32]byte]bool, len(roots))
	for _, root := range roots {
		set[root] = true
	}
	return set
}

func validateFetchedBlockRoot(
	block *types.SignedBlock,
	requestedRoots map[[32]byte]bool,
	seenRoots map[[32]byte]bool,
) error {
	if block == nil || block.Block == nil {
		return fmt.Errorf("malformed block")
	}
	root, err := block.Block.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("compute block root: %w", err)
	}
	if !requestedRoots[root] {
		return fmt.Errorf("peer returned unrequested block root 0x%x", root)
	}
	if seenRoots[root] {
		return fmt.Errorf("peer returned duplicate block root 0x%x", root)
	}
	seenRoots[root] = true
	return nil
}

func validateRootCount(count int) error {
	if count == 0 || count > int(types.MaxRequestBlocks) {
		return fmt.Errorf("blocks_by_root: root count %d outside range [1,%d]", count, types.MaxRequestBlocks)
	}
	return nil
}
