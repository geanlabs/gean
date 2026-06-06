package types

import "encoding/binary"

const (
	signaturePathOffset         uint32 = 36
	signatureHashesOffset       uint32 = 1064
	signaturePathSiblingsOffset uint32 = 4
)

func BlankXMSSSignature() [SignatureSize]byte {
	var sig [SignatureSize]byte
	binary.LittleEndian.PutUint32(sig[0:4], signaturePathOffset)
	binary.LittleEndian.PutUint32(sig[32:36], signatureHashesOffset)
	binary.LittleEndian.PutUint32(sig[36:40], signaturePathSiblingsOffset)
	return sig
}
