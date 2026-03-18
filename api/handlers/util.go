package handlers

import "encoding/hex"

func hex32(root [32]byte) string {
	return "0x" + hex.EncodeToString(root[:])
}
