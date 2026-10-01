//go:build tamago && riscv64 && zkvm_zisk

package sha256

// sha256fParams is the argument of ZisK's SHA-256 precompile: the state as
// eight native-order uint32 words and one raw 64-byte block, both 8-byte
// aligned.
type sha256fParams struct {
	state *[4]uint64
	block *[8]uint64
}

func compress(h *[4]uint64, block *[8]uint64) {
	p := sha256fParams{h, block}
	sha256f(&p)
}

// sha256f compresses one block into the state in place (defined in
// compress_zisk_riscv64.s).
//
//go:noescape
func sha256f(p *sha256fParams)
