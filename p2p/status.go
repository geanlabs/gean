package p2p

import (
	"context"
	"fmt"
	"github.com/geanlabs/gean/types"
	"io"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/geanlabs/gean/logger"
)

func (h *Host) handleStatusRequest(stream network.Stream, statusFn func() *types.Status) {
	armReadDeadline(stream)
	reqBuf, err := io.ReadAll(io.LimitReader(stream, int64(MaxCompressedPayloadSize)))
	if err != nil {
		logger.Warn(logger.Network, "status: read request failed: %v", err)
		return
	}
	if len(reqBuf) == 0 {
		h.writeResponse(stream, "status", RespInvalidRequest, []byte("empty status request"))
		return
	}

	payload, err := DecodeReqRespPayload(reqBuf)
	if err != nil {
		logger.Warn(logger.Network, "status: decode request failed: %v", err)
		h.writeResponse(stream, "status", RespInvalidRequest, []byte("decode failed"))
		return
	}

	peerStatus := &types.Status{}
	if err := peerStatus.UnmarshalSSZ(payload); err != nil {
		logger.Warn(logger.Network, "status: ssz unmarshal failed: %v", err)
		h.writeResponse(stream, "status", RespInvalidRequest, []byte("ssz unmarshal failed"))
		return
	}

	logger.Info(logger.Network, "status: peer at slot %d finalized=%d", peerStatus.HeadSlot, peerStatus.FinalizedSlot)

	status := statusFn()
	if status == nil {
		h.writeResponse(stream, "status", RespServerError, []byte("status unavailable"))
		return
	}
	h.writeResponse(stream, "status", RespSuccess, status.MarshalSSZ())
}

func (h *Host) SendStatusRequest(ctx context.Context, peerID peer.ID, ourStatus *types.Status) (*types.Status, error) {
	if ourStatus == nil {
		return nil, fmt.Errorf("status request: nil local status")
	}

	ctx, cancel := context.WithTimeout(ctx, ReqRespTimeout)
	defer cancel()

	stream, err := h.host.NewStream(ctx, peerID, protocol.ID(StatusProtocol))
	if err != nil {
		return nil, fmt.Errorf("open status stream: %w", err)
	}
	defer stream.Close()

	armWriteDeadline(stream)
	reqBytes := EncodeReqRespPayload(ourStatus.MarshalSSZ())
	if h.Hooks.ReqRespRequestSize != nil {
		h.Hooks.ReqRespRequestSize("status", len(reqBytes))
	}
	if _, err := stream.Write(reqBytes); err != nil {
		return nil, fmt.Errorf("write status request: %w", err)
	}
	stream.CloseWrite()

	code, respData, err := h.readReqRespChunk(stream, stream, "status")
	if err != nil {
		return nil, fmt.Errorf("read status response: %w", err)
	}
	if code != RespSuccess {
		return nil, fmt.Errorf("status response error: code=%d", code)
	}

	peerStatus := &types.Status{}
	if err := peerStatus.UnmarshalSSZ(respData); err != nil {
		return nil, fmt.Errorf("unmarshal status: %w", err)
	}
	return peerStatus, nil
}
