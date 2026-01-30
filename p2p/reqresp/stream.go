package reqresp

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/devylongs/gean/types"
	"github.com/golang/snappy"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

const (
	ReadTimeout  = 10 * time.Second
	WriteTimeout = 10 * time.Second
	MaxMsgSize   = 10 * 1024 * 1024 // 10MB
)

// Response codes per spec
const (
	RespCodeSuccess     byte = 0x00
	RespCodeInvalidReq  byte = 0x01
	RespCodeServerError byte = 0x02
)

// StreamHandler manages request/response protocol streams.
type StreamHandler struct {
	host    host.Host
	handler *Handler
}

// NewStreamHandler creates a new stream handler.
func NewStreamHandler(h host.Host, handler *Handler) *StreamHandler {
	return &StreamHandler{
		host:    h,
		handler: handler,
	}
}

// RegisterProtocols registers all request/response protocol handlers.
func (s *StreamHandler) RegisterProtocols() {
	s.host.SetStreamHandler(protocol.ID(StatusProtocolV1), s.handleStatusStream)
	s.host.SetStreamHandler(protocol.ID(BlocksByRootProtocolV1), s.handleBlocksByRootStream)
}

// handleStatusStream handles incoming Status requests.
func (s *StreamHandler) handleStatusStream(stream network.Stream) {
	defer stream.Close()

	// Set read deadline
	_ = stream.SetReadDeadline(time.Now().Add(ReadTimeout))

	// Read and decompress request
	data, err := readMessage(stream)
	if err != nil {
		slog.Debug("handleStatusStream: failed to read message", "error", err)
		writeErrorResponse(stream, RespCodeInvalidReq)
		return
	}

	// Unmarshal SSZ
	var peerStatus types.Status
	if err := peerStatus.UnmarshalSSZ(data); err != nil {
		slog.Debug("handleStatusStream: failed to unmarshal", "error", err)
		writeErrorResponse(stream, RespCodeInvalidReq)
		return
	}

	// Process and generate response
	ourStatus := s.handler.HandleStatus(&peerStatus)

	// Marshal response
	respData, err := ourStatus.MarshalSSZ()
	if err != nil {
		slog.Debug("handleStatusStream: failed to marshal response", "error", err)
		writeErrorResponse(stream, RespCodeServerError)
		return
	}

	// Write response (writeSuccessResponse handles compression)
	_ = stream.SetWriteDeadline(time.Now().Add(WriteTimeout))
	if err := writeSuccessResponse(stream, respData); err != nil {
		slog.Debug("handleStatusStream: failed to write response", "error", err)
		return
	}
}

// handleBlocksByRootStream handles incoming BlocksByRoot requests.
func (s *StreamHandler) handleBlocksByRootStream(stream network.Stream) {
	defer stream.Close()

	// Set read deadline
	_ = stream.SetReadDeadline(time.Now().Add(ReadTimeout))

	// Read and decompress request
	data, err := readMessage(stream)
	if err != nil {
		writeErrorResponse(stream, RespCodeInvalidReq)
		return
	}

	// Unmarshal SSZ
	var request types.BlocksByRootRequest
	if err := request.UnmarshalSSZ(data); err != nil {
		writeErrorResponse(stream, RespCodeInvalidReq)
		return
	}

	// Process request using the handler
	response := s.handler.HandleBlocksByRoot(&request)

	// Write each block as a separate response chunk
	_ = stream.SetWriteDeadline(time.Now().Add(WriteTimeout))
	for _, block := range response.Blocks {
		blockData, err := block.MarshalSSZ()
		if err != nil {
			continue
		}
		writeSuccessResponse(stream, blockData)
	}
}

// SendStatus sends a Status request to a peer and returns their status.
func (s *StreamHandler) SendStatus(ctx context.Context, peerID peer.ID, status *types.Status) (*types.Status, error) {
	stream, err := s.host.NewStream(ctx, peerID, protocol.ID(StatusProtocolV1))
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	defer stream.Close()

	// Marshal request
	data, err := status.MarshalSSZ()
	if err != nil {
		return nil, fmt.Errorf("marshal status: %w", err)
	}

	// Write request (writeMessage handles compression)
	_ = stream.SetWriteDeadline(time.Now().Add(WriteTimeout))
	if err := writeMessage(stream, data); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// Close write side to signal end of request
	if err := stream.CloseWrite(); err != nil {
		return nil, fmt.Errorf("close write: %w", err)
	}

	// Read response (readResponse handles decompression)
	_ = stream.SetReadDeadline(time.Now().Add(ReadTimeout))
	respCode, respData, err := readResponse(stream)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if respCode != RespCodeSuccess {
		return nil, fmt.Errorf("peer returned error code %d", respCode)
	}

	// Unmarshal SSZ (already decompressed)
	var peerStatus types.Status
	if err := peerStatus.UnmarshalSSZ(respData); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	return &peerStatus, nil
}

// RequestBlocksByRoot requests blocks from a peer by their roots.
func (s *StreamHandler) RequestBlocksByRoot(ctx context.Context, peerID peer.ID, roots []types.Root) ([]*types.SignedBlock, error) {
	stream, err := s.host.NewStream(ctx, peerID, protocol.ID(BlocksByRootProtocolV1))
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	defer stream.Close()

	// Marshal request
	request := &types.BlocksByRootRequest{Roots: roots}
	data, err := request.MarshalSSZ()
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Write request (writeMessage handles compression)
	_ = stream.SetWriteDeadline(time.Now().Add(WriteTimeout))
	if err := writeMessage(stream, data); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// Close write side to signal end of request
	if err := stream.CloseWrite(); err != nil {
		return nil, fmt.Errorf("close write: %w", err)
	}

	// Read responses (one per block, each already decompressed by readResponse)
	var blocks []*types.SignedBlock
	_ = stream.SetReadDeadline(time.Now().Add(ReadTimeout))

	for {
		respCode, respData, err := readResponse(stream)
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		if respCode != RespCodeSuccess {
			continue
		}

		var block types.SignedBlock
		if err := block.UnmarshalSSZ(respData); err != nil {
			continue
		}
		blocks = append(blocks, &block)
	}

	return blocks, nil
}

// Helper functions for framed message I/O
// Per spec: varint length prefix + snappy frame compressed SSZ

// readMessage reads a varint-prefixed, snappy-framed message from the stream.
func readMessage(r io.Reader) ([]byte, error) {
	// Read all available data (up to max size)
	// In practice, the stream will be closed after the message
	buf := make([]byte, MaxMsgSize)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, err
	}
	buf = buf[:n]

	if len(buf) < 2 {
		return nil, fmt.Errorf("message too short")
	}

	// Decode varint length prefix (uncompressed size)
	uncompressedSize, varintLen := binary.Uvarint(buf)
	if varintLen <= 0 {
		return nil, fmt.Errorf("invalid varint")
	}

	if uncompressedSize > MaxMsgSize {
		return nil, fmt.Errorf("message too large: %d", uncompressedSize)
	}

	// Decompress snappy-framed data
	compressed := buf[varintLen:]
	decoded, err := snappy.Decode(nil, compressed)
	if err != nil {
		return nil, fmt.Errorf("snappy decode: %w", err)
	}

	if uint64(len(decoded)) != uncompressedSize {
		return nil, fmt.Errorf("size mismatch: expected %d, got %d", uncompressedSize, len(decoded))
	}

	return decoded, nil
}

// writeMessage writes a varint-prefixed, snappy-framed message to the stream.
func writeMessage(w io.Writer, data []byte) error {
	// Compress with snappy
	compressed := snappy.Encode(nil, data)

	// Write varint length prefix (uncompressed size)
	varintBuf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(varintBuf, uint64(len(data)))
	if _, err := w.Write(varintBuf[:n]); err != nil {
		return err
	}

	// Write compressed data
	_, err := w.Write(compressed)
	return err
}

// readResponse reads a response code followed by the message.
func readResponse(r io.Reader) (byte, []byte, error) {
	// Read response code (1 byte)
	codeBuf := make([]byte, 1)
	if _, err := io.ReadFull(r, codeBuf); err != nil {
		return 0, nil, err
	}

	// Read message
	data, err := readMessage(r)
	return codeBuf[0], data, err
}

// writeSuccessResponse writes a success response with data.
func writeSuccessResponse(w io.Writer, data []byte) error {
	if _, err := w.Write([]byte{RespCodeSuccess}); err != nil {
		return err
	}
	return writeMessage(w, data)
}

// writeErrorResponse writes an error response code.
func writeErrorResponse(w io.Writer, code byte) error {
	_, err := w.Write([]byte{code})
	return err
}
