package statetransition

import "github.com/geanlabs/gean/internal/types"

func copyCheckpoint(cp *types.Checkpoint) *types.Checkpoint {
	if cp == nil {
		return nil
	}
	out := *cp
	return &out
}

func copyRootBytes(root [types.RootSize]byte) []byte {
	out := make([]byte, types.RootSize)
	copy(out, root[:])
	return out
}
