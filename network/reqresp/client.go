package reqresp

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/geanlabs/gean/types"
)

// ErrNoStatusResponse indicates that the remote peer closed the status stream
// without sending any response bytes.
var ErrNoStatusResponse = errors.New("status response missing")

// RequestStatus sends a status request to a peer and returns their response.
func RequestStatus(ctx context.Context, h host.Host, pid peer.ID, status Status) (*Status, error) {
	ctx, cancel := context.WithTimeout(ctx, reqRespTimeout)
	defer cancel()

	s, err := h.NewStream(ctx, pid, protocol.ID(StatusProtocol))
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	defer s.Close()

	if err := WriteStatus(s, status); err != nil {
		return nil, fmt.Errorf("write status: %w", err)
	}
	if err := s.CloseWrite(); err != nil {
		return nil, fmt.Errorf("close write: %w", err)
	}

	firstByte, err := ReadResponseCode(s)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, ErrNoStatusResponse
		}
		return nil, fmt.Errorf("read response code: %w", err)
	}

	// Interop fallback: some peers may send status payloads without the
	// response-code prefix.
	if !isKnownResponseCode(firstByte) {
		resp, err := ReadStatus(io.MultiReader(bytes.NewReader([]byte{firstByte}), s))
		if err != nil {
			return nil, fmt.Errorf("read response (no status code mode): %w", err)
		}
		return &resp, nil
	}
	if firstByte != ResponseSuccess {
		return nil, fmt.Errorf("peer returned error code %d", firstByte)
	}

	resp, err := ReadStatus(s)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return &resp, nil
}

// RequestBlocksByRoot requests blocks by their roots from a peer.
func RequestBlocksByRoot(ctx context.Context, h host.Host, pid peer.ID, roots [][32]byte) ([]*types.SignedBlockWithAttestation, error) {
	return requestBlocksByRootWithPayload(ctx, h, pid, encodeBlocksByRootRequest(roots))
}

func requestBlocksByRootWithPayload(
	ctx context.Context,
	h host.Host,
	pid peer.ID,
	payload []byte,
) ([]*types.SignedBlockWithAttestation, error) {
	ctx, cancel := context.WithTimeout(ctx, reqRespTimeout)
	defer cancel()

	s, err := h.NewStream(ctx, pid, protocol.ID(BlocksByRootProtocol), protocol.ID(BlocksByRootProtocolLegacy))
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	defer s.Close()

	// Write pre-encoded request payload.
	if err := WriteSnappyFrame(s, payload); err != nil {
		return nil, fmt.Errorf("write roots: %w", err)
	}
	if err := s.CloseWrite(); err != nil {
		return nil, fmt.Errorf("close write: %w", err)
	}

	// Read block responses until EOF. Each response is prefixed with a status byte.
	var blocks []*types.SignedBlockWithAttestation
	firstCode, err := ReadResponseCode(s)
	if err != nil {
		if err == io.EOF {
			return blocks, nil
		}
		return blocks, fmt.Errorf("read response code: %w", err)
	}

	// Interop fallback: some peers stream raw snappy frames without
	// per-chunk response codes. If the first byte is not a known response code,
	// treat it as the first byte of the frame varint length prefix.
	if !isKnownResponseCode(firstCode) {
		blocks, err := readFramedBlocks(io.MultiReader(bytes.NewReader([]byte{firstCode}), s))
		if err != nil {
			return nil, fmt.Errorf("read framed blocks (no status byte mode): %w", err)
		}
		return blocks, nil
	}

	code := firstCode
	for {
		if code != ResponseSuccess {
			return blocks, fmt.Errorf("peer returned blocks_by_root error code %d", code)
		}
		data, err := ReadSnappyFrame(s)
		if err != nil {
			return blocks, fmt.Errorf("read block: %w", err)
		}
		block := new(types.SignedBlockWithAttestation)
		if err := block.UnmarshalSSZ(data); err == nil {
			blocks = append(blocks, block)
		}

		code, err = ReadResponseCode(s)
		if err != nil {
			if err == io.EOF {
				break
			}
			return blocks, fmt.Errorf("read response code: %w", err)
		}
	}
	return blocks, nil
}

// encodeBlocksByRootRequest SSZ-encodes a BlocksByRootRequest container.
// The spec defines BlocksByRootRequest as a single-field Container:
//
//	class BlocksByRootRequest(Container):
//	    roots: RequestedBlockRoots  # SSZList[Bytes32]
//
// A variable-size field in an SSZ container is preceded by a 4-byte
// little-endian offset. With one field the offset is always 4.
// Wire layout: [offset=4 (4 bytes LE)][root_0 (32 bytes)]...[root_N (32 bytes)]
func encodeBlocksByRootRequest(roots [][32]byte) []byte {
	out := make([]byte, 4+len(roots)*32)
	binary.LittleEndian.PutUint32(out[:4], 4)
	for i, r := range roots {
		copy(out[4+i*32:], r[:])
	}
	return out
}

func readFramedBlocks(r io.Reader) ([]*types.SignedBlockWithAttestation, error) {
	var blocks []*types.SignedBlockWithAttestation
	for {
		data, err := ReadSnappyFrame(r)
		if err != nil {
			if err == io.EOF {
				break
			}
			return blocks, err
		}
		block := new(types.SignedBlockWithAttestation)
		if err := block.UnmarshalSSZ(data); err == nil {
			blocks = append(blocks, block)
		}
	}
	return blocks, nil
}

func isKnownResponseCode(code byte) bool {
	return code == ResponseSuccess ||
		code == ResponseInvalidRequest ||
		code == ResponseServerError ||
		code == ResponseResourceUnavailable
}
