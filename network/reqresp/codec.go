package reqresp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/golang/snappy"

	"github.com/geanlabs/gean/types"
)

// ReadStatus reads and decodes a snappy-framed status message.
func ReadStatus(r io.Reader) (Status, error) {
	data, err := ReadSnappyFrame(r)
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

// WriteStatus encodes and writes a snappy-framed status message.
func WriteStatus(w io.Writer, status Status) error {
	var buf [80]byte
	copy(buf[0:32], status.Finalized.Root[:])
	binary.LittleEndian.PutUint64(buf[32:40], status.Finalized.Slot)
	copy(buf[40:72], status.Head.Root[:])
	binary.LittleEndian.PutUint64(buf[72:80], status.Head.Slot)
	return WriteSnappyFrame(w, buf[:])
}

func writeSignedBlock(w io.Writer, block *types.SignedBlockWithAttestation) error {
	data, err := block.MarshalSSZ()
	if err != nil {
		return err
	}
	return WriteSnappyFrame(w, data)
}

func readBlocksByRootRequest(r io.Reader) ([][32]byte, error) {
	data, err := ReadSnappyFrame(r)
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

// ReadResponseCode reads a single response status byte.
func ReadResponseCode(r io.Reader) (byte, error) {
	var buf [1]byte
	_, err := io.ReadFull(r, buf[:])
	return buf[0], err
}

// ReadSnappyFrame reads a varint-length-prefixed snappy frame encoded message.
// Wire format: varint(uncompressed_len) + snappy_frame(data)
func ReadSnappyFrame(r io.Reader) ([]byte, error) {
	length, err := binary.ReadUvarint(byteReader{r})
	if err != nil {
		return nil, err
	}
	if length > 10*1024*1024 {
		return nil, fmt.Errorf("message too large: %d", length)
	}
	sr := snappy.NewReader(r)
	decoded := make([]byte, length)
	if _, err := io.ReadFull(sr, decoded); err != nil {
		return nil, fmt.Errorf("snappy frame decode: %w", err)
	}
	return decoded, nil
}

// WriteSnappyFrame writes a varint-length-prefixed snappy frame encoded message.
// Wire format: varint(uncompressed_len) + snappy_frame(data)
func WriteSnappyFrame(w io.Writer, data []byte) error {
	var lenBuf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(lenBuf[:], uint64(len(data)))
	if _, err := w.Write(lenBuf[:n]); err != nil {
		return err
	}
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
