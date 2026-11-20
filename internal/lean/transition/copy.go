package transition

import "github.com/geanlabs/gean/internal/lean/types"

func copyRootBytes(root [types.RootSize]byte) []byte {
	out := make([]byte, types.RootSize)
	copy(out, root[:])
	return out
}
