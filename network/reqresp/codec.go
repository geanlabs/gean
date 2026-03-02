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
	// devnet-2 canonical format: raw SSZList[Bytes32] payload.
	if len(data)%32 == 0 {
		return decodeRootsRaw(data)
	}

	// Interop fallback: some clients may encode BlocksByRootRequest as a
	// single-field container with a 4-byte offset, then the raw roots list.
	// Container layout for one dynamic field is:
	//   [offset(4 bytes little-endian)][roots-bytes...]
	if len(data) >= 4 {
		offset := binary.LittleEndian.Uint32(data[:4])
		if offset == 4 {
			rootsData := data[4:]
			if len(rootsData)%32 == 0 {
				return decodeRootsRaw(rootsData)
			}
		}
	}

	return nil, fmt.Errorf("invalid roots length: %d", len(data))
}

func decodeRootsRaw(data []byte) ([][32]byte, error) {
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
// Wire format: varint(uncompressed_len) + snappy_framed(data)
// The varint encodes the expected uncompressed byte length.
func ReadSnappyFrame(r io.Reader) ([]byte, error) {
	uncompressedLen, err := binary.ReadUvarint(byteReader{r})
	if err != nil {
		return nil, err
	}
	if uncompressedLen > 10*1024*1024 {
		return nil, fmt.Errorf("message too large: %d", uncompressedLen)
	}

	framed, err := readSnappyFramedStream(r, int(uncompressedLen))
	if err != nil {
		return nil, err
	}
	sr := snappy.NewReader(bytes.NewReader(framed))
	decoded, err := io.ReadAll(sr)
	if err != nil {
		return nil, fmt.Errorf("snappy frame decode: %w", err)
	}
	if len(decoded) != int(uncompressedLen) {
		return nil, fmt.Errorf("decoded length mismatch: got %d want %d", len(decoded), uncompressedLen)
	}
	return decoded, nil
}

// WriteSnappyFrame writes a varint-length-prefixed snappy frame encoded message.
// Wire format: varint(uncompressed_len) + snappy_framed(data)
// The varint encodes the uncompressed byte length.
func WriteSnappyFrame(w io.Writer, data []byte) error {
	var buf bytes.Buffer
	sw := snappy.NewBufferedWriter(&buf)
	if _, err := sw.Write(data); err != nil {
		return err
	}
	if err := sw.Close(); err != nil {
		return err
	}
	var lenBuf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(lenBuf[:], uint64(len(data)))
	if _, err := w.Write(lenBuf[:n]); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

func readSnappyFramedStream(r io.Reader, expectedUncompressed int) ([]byte, error) {
	var framed bytes.Buffer
	produced := 0

	for produced < expectedUncompressed {
		var hdr [4]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return nil, fmt.Errorf("read snappy chunk header: %w", err)
		}
		chunkType := hdr[0]
		chunkLen := int(hdr[1]) | int(hdr[2])<<8 | int(hdr[3])<<16
		if chunkLen < 0 || chunkLen > 1<<20 {
			return nil, fmt.Errorf("invalid snappy chunk length: %d", chunkLen)
		}

		chunk := make([]byte, chunkLen)
		if _, err := io.ReadFull(r, chunk); err != nil {
			return nil, fmt.Errorf("read snappy chunk payload: %w", err)
		}

		framed.Write(hdr[:])
		framed.Write(chunk)

		switch chunkType {
		case 0x00: // compressed data chunk
			if chunkLen < 4 {
				return nil, fmt.Errorf("compressed snappy chunk too short")
			}
			decodedLen, err := snappy.DecodedLen(chunk[4:])
			if err != nil {
				return nil, fmt.Errorf("snappy decoded length: %w", err)
			}
			produced += decodedLen
		case 0x01: // uncompressed data chunk
			if chunkLen < 4 {
				return nil, fmt.Errorf("uncompressed snappy chunk too short")
			}
			produced += chunkLen - 4
		case 0xff: // stream identifier
			// no produced increment
		default:
			// 0x80-0xfe are skippable by spec; others are unsupported here.
			if chunkType >= 0x80 {
				// no produced increment
				continue
			}
			return nil, fmt.Errorf("unknown unskippable snappy chunk type: 0x%02x", chunkType)
		}
	}
	return framed.Bytes(), nil
}

// byteReader wraps an io.Reader to implement io.ByteReader.
type byteReader struct {
	io.Reader
}

func (br byteReader) ReadByte() (byte, error) {
	var buf [1]byte
	_, err := io.ReadFull(br.Reader, buf[:])
	return buf[0], err
}
