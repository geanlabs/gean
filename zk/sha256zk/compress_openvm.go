//go:build tamago && riscv64 && zkvm_openvm

package sha256

// compress compresses one block into the state in place with OpenVM's SHA-256
// instruction (defined in compress_openvm_riscv64.s): the state is eight
// native-order uint32 words and the block is raw bytes.
//
//go:noescape
func compress(h *[4]uint64, block *[8]uint64)
