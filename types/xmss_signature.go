package types

import "encoding/binary"

// SSZ offsets inside an XMSS Signature container. Although parent containers
// inline XmssSignature as an opaque SignatureSize-byte blob, consumers that
// decode the inner Signature container expect the leading bytes to be valid
// SSZ offsets — not zero. Filling the buffer with all zeros produces an SSZ-
// invalid encoding that fails inner-container decoding.
//
// Container layout (matches leanSpec subspecs/xmss/containers.py Signature):
//
//	field        type                          size
//	path         HashTreeOpening (variable)    offset at byte 0
//	rho          Vector[Fp, RAND_LEN_FE]       28 bytes inline at bytes 4..32
//	hashes       List[HashDigest, ...]         offset at byte 32
//
// path body = 4-byte siblings offset + 32 siblings × 32-byte digest = 1028.
// path starts at byte 36, hashes start at 36 + 1028 = 1064.
// path's siblings list body starts at offset 4 within the path container.
const (
	signaturePathOffset         uint32 = 36
	signatureHashesOffset       uint32 = 1064
	signaturePathSiblingsOffset uint32 = 4
)

// BlankXMSSSignature returns an SSZ-structurally-valid placeholder XMSS
// signature for anchor blocks that were never proposed (genesis, checkpoint-
// sync anchor served as opaque body). Decodes back to a Signature container
// of all-zero hashes:
//
//	Signature{path = HashTreeOpening{siblings = [0; 32]}, rho = 0, hashes = [0; 46]}
//
// Wire format matches ream's blank() and ethlambda's blank_xmss_signature(),
// so the placeholder is byte-identical across clients on BlocksByRoot.
func BlankXMSSSignature() [SignatureSize]byte {
	var sig [SignatureSize]byte
	binary.LittleEndian.PutUint32(sig[0:4], signaturePathOffset)
	binary.LittleEndian.PutUint32(sig[32:36], signatureHashesOffset)
	binary.LittleEndian.PutUint32(sig[36:40], signaturePathSiblingsOffset)
	return sig
}
