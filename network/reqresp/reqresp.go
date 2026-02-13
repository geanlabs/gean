package reqresp

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/golang/snappy"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/devylongs/gean/types"
)

// Protocol IDs matching leanSpec devnet0.
const (
	StatusProtocol       = "/leanconsensus/req/status/1/"
	BlocksByRootProtocol = "/leanconsensus/req/blocks_by_root/1/"
)

const reqRespTimeout = 10 * time.Second

// Status is the status message exchanged between peers.
type Status struct {
	Finalized *types.Checkpoint
	Head      *types.Checkpoint
}

// ReqRespHandler processes incoming request/response messages.
type ReqRespHandler struct {
	OnStatus       func(Status) Status
	OnBlocksByRoot func([][32]byte) []*types.SignedBlock
}

// RegisterReqResp registers request/response protocol handlers.
func RegisterReqResp(h host.Host, handler *ReqRespHandler) {
	h.SetStreamHandler(StatusProtocol, func(s network.Stream) {
		defer s.Close()
		handleStatus(s, handler)
	})

	h.SetStreamHandler(BlocksByRootProtocol, func(s network.Stream) {
		defer s.Close()
		handleBlocksByRoot(s, handler)
	})
}

func handleStatus(s network.Stream, handler *ReqRespHandler) {
	if handler.OnStatus == nil {
		return
	}
	req, err := readStatus(s)
	if err != nil {
		return
	}
	resp := handler.OnStatus(req)
	if err := writeStatus(s, resp); err != nil {
		return
	}
}

func handleBlocksByRoot(s network.Stream, handler *ReqRespHandler) {
	if handler.OnBlocksByRoot == nil {
		return
	}
	roots, err := readBlocksByRootRequest(s)
	if err != nil {
		return
	}
	blocks := handler.OnBlocksByRoot(roots)
	for _, block := range blocks {
		if err := writeSignedBlock(s, block); err != nil {
			return
		}
	}
}

// RequestStatus sends a status request to a peer and returns their response.
func RequestStatus(ctx context.Context, h host.Host, pid peer.ID, status Status) (*Status, error) {
	ctx, cancel := context.WithTimeout(ctx, reqRespTimeout)
	defer cancel()

	s, err := h.NewStream(ctx, pid, protocol.ID(StatusProtocol))
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	defer s.Close()

	if err := writeStatus(s, status); err != nil {
		return nil, fmt.Errorf("write status: %w", err)
	}
	if err := s.CloseWrite(); err != nil {
		return nil, fmt.Errorf("close write: %w", err)
	}

	resp, err := readStatus(s)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return &resp, nil
}

// RequestBlocksByRoot requests blocks by their roots from a peer.
func RequestBlocksByRoot(ctx context.Context, h host.Host, pid peer.ID, roots [][32]byte) ([]*types.SignedBlock, error) {
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
	if err := writeSnappyFrame(s, rootsBuf); err != nil {
		return nil, fmt.Errorf("write roots: %w", err)
	}
	if err := s.CloseWrite(); err != nil {
		return nil, fmt.Errorf("close write: %w", err)
	}

	// Read block responses until EOF.
	var blocks []*types.SignedBlock
	for {
		data, err := readSnappyFrame(s)
		if err != nil {
			if err == io.EOF {
				break
			}
			return blocks, fmt.Errorf("read block: %w", err)
		}
		block := new(types.SignedBlock)
		if err := block.UnmarshalSSZ(data); err != nil {
			continue
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func readStatus(r io.Reader) (Status, error) {
	data, err := readSnappyFrame(r)
	if err != nil {
		return Status{}, err
	}
	if len(data) != 80 {
		return Status{}, fmt.Errorf("invalid status length: %d", len(data))
	}
	finalized := &types.Checkpoint{Slot: binary.LittleEndian.Uint64(data[32:40])}
	copy(finalized.Root[:], data[0:32])
	head := &types.Checkpoint{Slot: binary.LittleEndian.Uint64(data[72:80])}
	copy(head.Root[:], data[40:72])
	return Status{Finalized: finalized, Head: head}, nil
}

func writeStatus(w io.Writer, status Status) error {
	var buf [80]byte
	copy(buf[0:32], status.Finalized.Root[:])
	binary.LittleEndian.PutUint64(buf[32:40], status.Finalized.Slot)
	copy(buf[40:72], status.Head.Root[:])
	binary.LittleEndian.PutUint64(buf[72:80], status.Head.Slot)
	return writeSnappyFrame(w, buf[:])
}

func writeSignedBlock(w io.Writer, block *types.SignedBlock) error {
	data, err := block.MarshalSSZ()
	if err != nil {
		return err
	}
	return writeSnappyFrame(w, data)
}

func readBlocksByRootRequest(r io.Reader) ([][32]byte, error) {
	data, err := readSnappyFrame(r)
	if err != nil {
		return nil, err
	}
	if len(data)%32 != 0 {
		return nil, fmt.Errorf("invalid roots length: %d", len(data))
	}
	n := len(data) / 32
	if n > types.MaxRequestBlocks {
		return nil, fmt.Errorf("too many roots: %d", n)
	}
	roots := make([][32]byte, n)
	for i := range roots {
		copy(roots[i][:], data[i*32:(i+1)*32])
	}
	return roots, nil
}

// readSnappyFrame reads a varint-length-prefixed snappy frame encoded message.
// Wire format: varint(uncompressed_len) + snappy_frame(data)
func readSnappyFrame(r io.Reader) ([]byte, error) {
	// Read varint: the uncompressed SSZ length.
	length, err := binary.ReadUvarint(byteReader{r})
	if err != nil {
		return nil, err
	}
	if length > 10*1024*1024 {
		return nil, fmt.Errorf("message too large: %d", length)
	}
	// Decompress through snappy frame reader.
	sr := snappy.NewReader(r)
	decoded := make([]byte, length)
	if _, err := io.ReadFull(sr, decoded); err != nil {
		return nil, fmt.Errorf("snappy frame decode: %w", err)
	}
	return decoded, nil
}

// writeSnappyFrame writes a varint-length-prefixed snappy frame encoded message.
// Wire format: varint(uncompressed_len) + snappy_frame(data)
func writeSnappyFrame(w io.Writer, data []byte) error {
	// Write varint of uncompressed length.
	var lenBuf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(lenBuf[:], uint64(len(data)))
	if _, err := w.Write(lenBuf[:n]); err != nil {
		return err
	}
	// Compress via snappy frame writer.
	var buf bytes.Buffer
	sw := snappy.NewBufferedWriter(&buf)
	if _, err := sw.Write(data); err != nil {
		return err
	}
	if err := sw.Close(); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// byteReader wraps an io.Reader to implement io.ByteReader.
type byteReader struct {
	io.Reader
}

func (br byteReader) ReadByte() (byte, error) {
	var buf [1]byte
	_, err := br.Reader.Read(buf[:])
	return buf[0], err
}
