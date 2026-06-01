//go:build spectests

package spectests

import (
	"encoding/binary"
	"fmt"

	ssz "github.com/ferranbt/fastssz"
)

// XMSS container constants for the prod TARGET_CONFIG. Gean stores XMSS
// payloads as opaque byte arrays
// ([PubkeySize]byte, [SignatureSize]byte) because all real work lives in
// the Rust FFI; these adapters reconstruct the spec-defined inner structure
// solely to drive hash_tree_root for the spec SSZ conformance test.
const (
	xmssHashLenFE       = 8                           // Vector[Fp, 8] — 32 bytes
	xmssParameterLen    = 5                           // Vector[Fp, 5] — 20 bytes
	xmssRandLenFE       = 7                           // Vector[Fp, 7] — 28 bytes
	xmssFpBytes         = 4                           // KoalaBear Fp serialized width
	xmssHashDigestBytes = xmssHashLenFE * xmssFpBytes // 32

	// NODE_LIST_LIMIT = 1 << (LOG_LIFETIME/2 + 1) = 1 << 17 = 131072 for
	// LOG_LIFETIME = 32 (prod). Drives the SSZ list mix-in depth.
	xmssNodeListLimit uint64 = 1 << 17
)

// --- PublicKey -------------------------------------------------------------
//
// SSZ Container { root: Vector[Fp, 8], parameter: Vector[Fp, 5] }
// Both fields are fixed-size, so the wire form is plain concatenation:
//   32 bytes root + 20 bytes parameter = 52 bytes total.
// hash_tree_root: merkleize([root_chunk, parameter_chunk]) where each chunk
// is the bytes padded to 32.

type sszPublicKeyAdapter struct {
	Root      [xmssHashDigestBytes]byte
	Parameter [xmssParameterLen * xmssFpBytes]byte
}

const xmssPublicKeyBytes = xmssHashDigestBytes + xmssParameterLen*xmssFpBytes

func (p *sszPublicKeyAdapter) MarshalSSZ() ([]byte, error) {
	out := make([]byte, xmssPublicKeyBytes)
	copy(out[:xmssHashDigestBytes], p.Root[:])
	copy(out[xmssHashDigestBytes:], p.Parameter[:])
	return out, nil
}

func (p *sszPublicKeyAdapter) UnmarshalSSZ(buf []byte) error {
	if len(buf) != xmssPublicKeyBytes {
		return fmt.Errorf("PublicKey: want %d bytes, got %d", xmssPublicKeyBytes, len(buf))
	}
	copy(p.Root[:], buf[:xmssHashDigestBytes])
	copy(p.Parameter[:], buf[xmssHashDigestBytes:])
	return nil
}

func (p *sszPublicKeyAdapter) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(p)
}

func (p *sszPublicKeyAdapter) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(p)
}

func (p *sszPublicKeyAdapter) HashTreeRootWith(hh ssz.HashWalker) error {
	indx := hh.Index()
	hh.PutBytes(p.Root[:])      // 32 bytes — one chunk
	hh.PutBytes(p.Parameter[:]) // 20 bytes — zero-padded by hasher to one chunk
	hh.Merkleize(indx)
	return nil
}

// --- HashTreeOpening -------------------------------------------------------
//
// SSZ Container { siblings: List[Vector[Fp, 8], NODE_LIST_LIMIT] }
// Wire form: 4-byte offset (= 4) then concatenated 32-byte digests.
// hash_tree_root: merkleize([list_root]) where list_root mixes in the
// element count over NODE_LIST_LIMIT.

type sszHashTreeOpeningAdapter struct {
	Siblings [][xmssHashDigestBytes]byte
}

func (h *sszHashTreeOpeningAdapter) MarshalSSZ() ([]byte, error) {
	out := make([]byte, 4+len(h.Siblings)*xmssHashDigestBytes)
	binary.LittleEndian.PutUint32(out[:4], 4)
	for i, s := range h.Siblings {
		copy(out[4+i*xmssHashDigestBytes:], s[:])
	}
	return out, nil
}

func (h *sszHashTreeOpeningAdapter) UnmarshalSSZ(buf []byte) error {
	if len(buf) < 4 {
		return fmt.Errorf("HashTreeOpening: buffer too short for offset")
	}
	off := binary.LittleEndian.Uint32(buf[:4])
	if off != 4 {
		return fmt.Errorf("HashTreeOpening: unexpected offset %d", off)
	}
	rest := buf[4:]
	if len(rest)%xmssHashDigestBytes != 0 {
		return fmt.Errorf("HashTreeOpening: siblings tail %d not divisible by digest size %d",
			len(rest), xmssHashDigestBytes)
	}
	n := len(rest) / xmssHashDigestBytes
	h.Siblings = make([][xmssHashDigestBytes]byte, n)
	for i := 0; i < n; i++ {
		copy(h.Siblings[i][:], rest[i*xmssHashDigestBytes:(i+1)*xmssHashDigestBytes])
	}
	return nil
}

func (h *sszHashTreeOpeningAdapter) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(h)
}

func (h *sszHashTreeOpeningAdapter) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(h)
}

func (h *sszHashTreeOpeningAdapter) HashTreeRootWith(hh ssz.HashWalker) error {
	indx := hh.Index()
	{
		subIndx := hh.Index()
		for i := range h.Siblings {
			hh.PutBytes(h.Siblings[i][:])
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(h.Siblings)), xmssNodeListLimit)
	}
	hh.Merkleize(indx)
	return nil
}

// --- HashTreeLayer ---------------------------------------------------------
//
// SSZ Container { start_index: uint64, nodes: List[Vector[Fp, 8], NODE_LIST_LIMIT] }
// Wire form: 8-byte start_index + 4-byte offset (=12) + concatenated nodes.
// hash_tree_root: merkleize([start_index_chunk, list_root]).

type sszHashTreeLayerAdapter struct {
	StartIndex uint64
	Nodes      [][xmssHashDigestBytes]byte
}

const xmssHashTreeLayerHeader = 8 + 4 // start_index + offset

func (h *sszHashTreeLayerAdapter) MarshalSSZ() ([]byte, error) {
	out := make([]byte, xmssHashTreeLayerHeader+len(h.Nodes)*xmssHashDigestBytes)
	binary.LittleEndian.PutUint64(out[:8], h.StartIndex)
	binary.LittleEndian.PutUint32(out[8:12], xmssHashTreeLayerHeader)
	for i, n := range h.Nodes {
		copy(out[xmssHashTreeLayerHeader+i*xmssHashDigestBytes:], n[:])
	}
	return out, nil
}

func (h *sszHashTreeLayerAdapter) UnmarshalSSZ(buf []byte) error {
	if len(buf) < xmssHashTreeLayerHeader {
		return fmt.Errorf("HashTreeLayer: buffer too short for header")
	}
	h.StartIndex = binary.LittleEndian.Uint64(buf[:8])
	off := binary.LittleEndian.Uint32(buf[8:12])
	if uint64(off) != uint64(xmssHashTreeLayerHeader) {
		return fmt.Errorf("HashTreeLayer: unexpected nodes offset %d", off)
	}
	rest := buf[xmssHashTreeLayerHeader:]
	if len(rest)%xmssHashDigestBytes != 0 {
		return fmt.Errorf("HashTreeLayer: nodes tail %d not divisible by digest size %d",
			len(rest), xmssHashDigestBytes)
	}
	n := len(rest) / xmssHashDigestBytes
	h.Nodes = make([][xmssHashDigestBytes]byte, n)
	for i := 0; i < n; i++ {
		copy(h.Nodes[i][:], rest[i*xmssHashDigestBytes:(i+1)*xmssHashDigestBytes])
	}
	return nil
}

func (h *sszHashTreeLayerAdapter) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(h)
}

func (h *sszHashTreeLayerAdapter) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(h)
}

func (h *sszHashTreeLayerAdapter) HashTreeRootWith(hh ssz.HashWalker) error {
	indx := hh.Index()
	hh.PutUint64(h.StartIndex)
	{
		subIndx := hh.Index()
		for i := range h.Nodes {
			hh.PutBytes(h.Nodes[i][:])
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(h.Nodes)), xmssNodeListLimit)
	}
	hh.Merkleize(indx)
	return nil
}

// --- Signature -------------------------------------------------------------
//
// SSZ Container { path: HashTreeOpening, rho: Vector[Fp, 7], hashes: HashDigestList }
// path and hashes are variable-size, so the wire form is the standard
// offsets-first layout: off_path (4) + rho (28) + off_hashes (4) + path body
// + hashes body. The spec overrides is_fixed_size to true and reports
// SIGNATURE_LEN_BYTES = 2536, so parent containers inline the bytes; the
// wire format is still the SSZ offset/body layout above.
// hash_tree_root: merkleize([htr(path), rho_chunk, htr(hashes)]).

type sszSignatureAdapter struct {
	Path   sszHashTreeOpeningAdapter
	Rho    [xmssRandLenFE * xmssFpBytes]byte
	Hashes [][xmssHashDigestBytes]byte
}

func (s *sszSignatureAdapter) MarshalSSZ() ([]byte, error) {
	pathBytes, err := s.Path.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	hashesBody := make([]byte, len(s.Hashes)*xmssHashDigestBytes)
	for i, h := range s.Hashes {
		copy(hashesBody[i*xmssHashDigestBytes:], h[:])
	}

	fixedHeader := 4 + len(s.Rho) + 4
	offPath := uint32(fixedHeader)
	offHashes := uint32(fixedHeader + len(pathBytes))

	out := make([]byte, fixedHeader+len(pathBytes)+len(hashesBody))
	binary.LittleEndian.PutUint32(out[:4], offPath)
	copy(out[4:4+len(s.Rho)], s.Rho[:])
	binary.LittleEndian.PutUint32(out[4+len(s.Rho):fixedHeader], offHashes)
	copy(out[fixedHeader:fixedHeader+len(pathBytes)], pathBytes)
	copy(out[fixedHeader+len(pathBytes):], hashesBody)
	return out, nil
}

func (s *sszSignatureAdapter) UnmarshalSSZ(buf []byte) error {
	fixedHeader := 4 + len(s.Rho) + 4
	if len(buf) < fixedHeader {
		return fmt.Errorf("Signature: buffer too short for header")
	}
	offPath := binary.LittleEndian.Uint32(buf[:4])
	copy(s.Rho[:], buf[4:4+len(s.Rho)])
	offHashes := binary.LittleEndian.Uint32(buf[4+len(s.Rho) : fixedHeader])

	if uint64(offPath) != uint64(fixedHeader) {
		return fmt.Errorf("Signature: unexpected path offset %d", offPath)
	}
	if offHashes < offPath || uint64(offHashes) > uint64(len(buf)) {
		return fmt.Errorf("Signature: invalid hashes offset %d (path=%d, total=%d)",
			offHashes, offPath, len(buf))
	}

	pathBytes := buf[offPath:offHashes]
	if err := s.Path.UnmarshalSSZ(pathBytes); err != nil {
		return fmt.Errorf("Signature.path: %w", err)
	}

	hashesBytes := buf[offHashes:]
	if len(hashesBytes)%xmssHashDigestBytes != 0 {
		return fmt.Errorf("Signature.hashes tail %d not divisible by digest size %d",
			len(hashesBytes), xmssHashDigestBytes)
	}
	n := len(hashesBytes) / xmssHashDigestBytes
	s.Hashes = make([][xmssHashDigestBytes]byte, n)
	for i := 0; i < n; i++ {
		copy(s.Hashes[i][:], hashesBytes[i*xmssHashDigestBytes:(i+1)*xmssHashDigestBytes])
	}
	return nil
}

func (s *sszSignatureAdapter) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(s)
}

func (s *sszSignatureAdapter) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(s)
}

func (s *sszSignatureAdapter) HashTreeRootWith(hh ssz.HashWalker) error {
	indx := hh.Index()
	if err := s.Path.HashTreeRootWith(hh); err != nil {
		return err
	}
	hh.PutBytes(s.Rho[:]) // 28 bytes — zero-padded by hasher to one 32-byte chunk
	{
		subIndx := hh.Index()
		for i := range s.Hashes {
			hh.PutBytes(s.Hashes[i][:])
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(s.Hashes)), xmssNodeListLimit)
	}
	hh.Merkleize(indx)
	return nil
}
