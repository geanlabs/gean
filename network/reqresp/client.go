package reqresp

import (
	"context"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/geanlabs/gean/types"
)

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

	code, err := ReadResponseCode(s)
	if err != nil {
		return nil, fmt.Errorf("read response code: %w", err)
	}
	if code != ResponseSuccess {
		return nil, fmt.Errorf("peer returned error code %d", code)
	}

	resp, err := ReadStatus(s)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return &resp, nil
}

// RequestBlocksByRoot requests blocks by their roots from a peer.
func RequestBlocksByRoot(ctx context.Context, h host.Host, pid peer.ID, roots [][32]byte) ([]*types.SignedBlockWithAttestation, error) {
	ctx, cancel := context.WithTimeout(ctx, reqRespTimeout)
	defer cancel()

	s, err := h.NewStream(ctx, pid, protocol.ID(BlocksByRootProtocol))
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	defer s.Close()

	// Write roots as concatenated 32-byte hashes.
	var rootsBuf []byte
	for _, r := range roots {
		rootsBuf = append(rootsBuf, r[:]...)
	}
	if err := WriteSnappyFrame(s, rootsBuf); err != nil {
		return nil, fmt.Errorf("write roots: %w", err)
	}
	if err := s.CloseWrite(); err != nil {
		return nil, fmt.Errorf("close write: %w", err)
	}

	// Read block responses until EOF. Each response is prefixed with a status byte.
	var blocks []*types.SignedBlockWithAttestation
	for {
		code, err := ReadResponseCode(s)
		if err != nil {
			if err == io.EOF {
				break
			}
			return blocks, fmt.Errorf("read response code: %w", err)
		}
		if code != ResponseSuccess {
			break
		}
		data, err := ReadSnappyFrame(s)
		if err != nil {
			return blocks, fmt.Errorf("read block: %w", err)
		}
		block := new(types.SignedBlockWithAttestation)
		if err := block.UnmarshalSSZ(data); err != nil {
			continue
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}
