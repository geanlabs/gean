// Package sha256 replaces github.com/minio/sha256-simd in zkVM guest builds
// (see the replace directive in zk/go.mod), so the SSZ hashing of the state
// transition runs each 64-byte block through the zkVM's SHA-256 precompile.
// It provides the part of sha256-simd's API that fastssz uses. Outside a
// zkVM guest, blocks are compressed in plain Go.
package sha256

import (
	"encoding/binary"
	"hash"
	"unsafe"
)

const (
	Size      = 32
	BlockSize = 64
)

var iv = [8]uint32{0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19}

// digest keeps the state and the pending block in uint64 arrays: every
// precompile needs 8-byte aligned operands. The state is eight uint32 words in
// native (little-endian) order, as every precompile reads it.
type digest struct {
	h   [4]uint64
	x   [8]uint64
	nx  int
	len uint64
}

// New returns a SHA-256 hash.Hash.
func New() hash.Hash {
	d := new(digest)
	d.Reset()
	return d
}

// Sum256 returns the SHA-256 digest of data.
func Sum256(data []byte) [Size]byte {
	var d digest
	d.Reset()
	d.Write(data)
	return d.sum()
}

func (d *digest) state() *[8]uint32 { return (*[8]uint32)(unsafe.Pointer(&d.h)) }

func (d *digest) block() *[BlockSize]byte { return (*[BlockSize]byte)(unsafe.Pointer(&d.x)) }

func (d *digest) Reset() {
	*d.state() = iv
	d.nx = 0
	d.len = 0
}

func (d *digest) Size() int { return Size }

func (d *digest) BlockSize() int { return BlockSize }

func (d *digest) Write(p []byte) (int, error) {
	n := len(p)
	d.len += uint64(n)
	for len(p) > 0 {
		c := copy(d.block()[d.nx:], p)
		d.nx += c
		p = p[c:]
		if d.nx == BlockSize {
			compress(&d.h, &d.x)
			d.nx = 0
		}
	}
	return n, nil
}

func (d *digest) Sum(in []byte) []byte {
	c := *d
	s := c.sum()
	return append(in, s[:]...)
}

func (d *digest) sum() [Size]byte {
	bits := d.len << 3
	var pad [BlockSize + 8]byte
	pad[0] = 0x80
	n := BlockSize - (d.nx+8)%BlockSize
	binary.BigEndian.PutUint64(pad[n:], bits)
	d.Write(pad[:n+8])

	var out [Size]byte
	for i, w := range d.state() {
		binary.BigEndian.PutUint32(out[4*i:], w)
	}
	return out
}
