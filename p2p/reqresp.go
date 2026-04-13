package p2p

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/types"
)

// Protocol IDs rs.
const (
	StatusProtocol       = "/leanconsensus/req/status/1/ssz_snappy"
	BlocksByRootProtocol = "/leanconsensus/req/blocks_by_root/1/ssz_snappy"
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

// RegisterReqRespHandlers registers stream handlers for status and blocks_by_root.
func (h *Host) RegisterReqRespHandlers(statusFn func() *StatusMessage, blockByRootFn func(root [32]byte) *types.SignedBlock) {
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
}

func handleStatusRequest(s network.Stream, statusFn func() *StatusMessage) {
	// Read request payload.
	reqBuf, err := io.ReadAll(io.LimitReader(s, int64(MaxCompressedPayloadSize)))
	if err != nil {
		logger.Warn(logger.Network, "status: read request failed: %v", err)
		return
	}

	if len(reqBuf) > 0 {
		// Decode peer's status (optional — we respond regardless).
		if payload, err := DecodeReqRespPayload(reqBuf); err == nil {
			peerStatus := &StatusMessage{}
			peerStatus.UnmarshalSSZ(payload)
			logger.Info(logger.Network, "status: peer at slot %d finalized=%d", peerStatus.HeadSlot, peerStatus.FinalizedSlot)
		}
	}

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

	logger.Info(logger.Network, "blocks_by_root: peer requested %d roots", len(roots))

	for _, root := range roots {
		block := blockByRootFn(root)
		if block == nil {
			continue // silently skip missing blocks (per spec)
		}

		blockData, err := block.MarshalSSZ()
		if err != nil {
			s.Write(EncodeResponse(RespServerError, []byte("marshal failed")))
			continue
		}

		s.Write(EncodeResponse(RespSuccess, blockData))
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
