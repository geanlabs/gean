package zkvectors

import (
	"fmt"
	"math/rand/v2"

	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/internal/zkstf"
)

// Input is one framed guest input. Its expected outcome is whatever the Go
// transition does with it; generators never predict it.
type Input struct {
	Name  string
	Bytes []byte
}

// field mutates one part of a pre-state or block so a second implementation
// is exercised on states and votes no honest chain produces.
type field struct {
	name   string
	mutate func(r *rand.Rand, pre *types.State, b *types.Block)
}

var fields = []field{
	{"source-slot", func(r *rand.Rand, _ *types.State, b *types.Block) { vote(r, b).Data.Source.Slot += nudge(r) }},
	{"target-slot", func(r *rand.Rand, _ *types.State, b *types.Block) { vote(r, b).Data.Target.Slot += nudge(r) }},
	{"head-root", func(r *rand.Rand, _ *types.State, b *types.Block) { vote(r, b).Data.Head.Root[r.IntN(32)] ^= 1 }},
	{"target-root", func(r *rand.Rand, _ *types.State, b *types.Block) { vote(r, b).Data.Target.Root[r.IntN(32)] ^= 1 }},
	{"source-root-zero", func(r *rand.Rand, _ *types.State, b *types.Block) { vote(r, b).Data.Source.Root = types.ZeroRoot }},
	{"bits", func(r *rand.Rand, pre *types.State, b *types.Block) {
		vote(r, b).AggregationBits = types.BitlistFromIndices([]uint64{uint64(r.IntN(int(pre.NumValidators()) + 2))})
	}},
	{"bits-unset", func(r *rand.Rand, _ *types.State, b *types.Block) {
		vote(r, b).AggregationBits = types.NewBitlistSSZ(uint64(1 + r.IntN(8)))
	}},
	{"duplicate-vote", func(r *rand.Rand, _ *types.State, b *types.Block) {
		b.Body.Attestations = append(b.Body.Attestations, vote(r, b))
	}},
	{"justified-slot", func(r *rand.Rand, pre *types.State, _ *types.Block) { pre.LatestJustified.Slot += nudge(r) }},
	{"finalized-slot", func(r *rand.Rand, pre *types.State, _ *types.Block) { pre.LatestFinalized.Slot += nudge(r) }},
	{"justified-slots", func(r *rand.Rand, pre *types.State, _ *types.Block) {
		pre.JustifiedSlots = types.NewBitlistSSZ(uint64(r.IntN(24)))
		for i := range types.BitlistLen(pre.JustifiedSlots) {
			if r.IntN(2) == 0 {
				types.BitlistSet(pre.JustifiedSlots, i)
			}
		}
	}},
	{"justification-root", func(r *rand.Rand, pre *types.State, _ *types.Block) {
		root := make([]byte, types.RootSize)
		if r.IntN(2) == 0 {
			root[0] = 1
		}
		pre.JustificationsRoots = append(pre.JustificationsRoots, root)
		n := uint64(len(pre.JustificationsRoots)) * pre.NumValidators()
		pre.JustificationsValidators = types.NewBitlistSSZ(n - uint64(r.IntN(2)))
	}},
	{"block-slot", func(r *rand.Rand, _ *types.State, b *types.Block) { b.Slot += uint64(1 + r.IntN(3)) }},
}

// vote returns one of the block's votes, adding a copy of the parent vote
// shape when the block carries none.
func vote(r *rand.Rand, b *types.Block) *types.AggregatedAttestation {
	if len(b.Body.Attestations) == 0 {
		cp := &types.Checkpoint{Root: b.ParentRoot, Slot: b.Slot - 1}
		b.Body.Attestations = append(b.Body.Attestations, &types.AggregatedAttestation{
			AggregationBits: types.BitlistFromIndices([]uint64{0}),
			Data:            &types.AttestationData{Slot: cp.Slot, Head: cp, Target: cp, Source: &types.Checkpoint{}},
		})
	}
	a := b.Body.Attestations[r.IntN(len(b.Body.Attestations))]
	data := *a.Data
	head, target, source := *data.Head, *data.Target, *data.Source
	data.Head, data.Target, data.Source = &head, &target, &source
	out := &types.AggregatedAttestation{AggregationBits: append([]byte(nil), a.AggregationBits...), Data: &data}
	for i, x := range b.Body.Attestations {
		if x == a {
			b.Body.Attestations[i] = out
		}
	}
	return out
}

func nudge(r *rand.Rand) uint64 { return []uint64{1, 2, ^uint64(0)}[r.IntN(3)] }

// Mutations derives n inputs from the honest cases: one or two field
// mutations, after which the block's state root is resealed when the mutated
// block still applies, so an accepted case shows through its post-state root
// whether each vote counted. Every fourth input is instead damaged at the byte
// level to exercise decoding.
func Mutations(cases []Case, seed uint64, n int) ([]Input, error) {
	r := rand.New(rand.NewPCG(seed, 0))
	var honest []Case
	for _, c := range cases {
		if !c.WantErr && c.Pre.NumValidators() < 64 {
			honest = append(honest, c)
		}
	}
	out := make([]Input, 0, n)
	for i := range n {
		c := honest[r.IntN(len(honest))]
		pre, err := c.Pre.Clone()
		if err != nil {
			return nil, err
		}
		block, err := cloneBlock(c.Block)
		if err != nil {
			return nil, err
		}
		name := fmt.Sprintf("mutate/%03d-%s", i, c.Name)
		if i%4 == 3 {
			in, err := zkstf.NewInput(pre, block)
			if err != nil {
				return nil, err
			}
			pos := r.IntN(len(in))
			if r.IntN(2) == 0 {
				in = in[:pos]
			} else {
				in[pos] ^= byte(1 + r.IntN(255))
			}
			out = append(out, Input{Name: name + "-bytes", Bytes: in})
			continue
		}
		for range 1 + r.IntN(2) {
			f := fields[r.IntN(len(fields))]
			f.mutate(r, pre, block)
			name += "-" + f.name
		}
		if err := seal(pre, block); err != nil {
			return nil, err
		}
		in, err := zkstf.NewInput(pre, block)
		if err != nil {
			return nil, err
		}
		out = append(out, Input{Name: name, Bytes: in})
	}
	return out, nil
}

// seal points the block at its parent as pre now hashes and names the
// expected proposer, then sets its state root to the post-state root when the
// block applies; a block that does not apply keeps its state root.
func seal(pre *types.State, block *types.Block) error {
	post, err := pre.Clone()
	if err != nil {
		return err
	}
	if statetransition.ProcessSlots(post, block.Slot) != nil {
		return nil
	}
	block.ProposerIndex = types.ProposerIndex(block.Slot, post.NumValidators())
	if block.ParentRoot, err = post.LatestBlockHeader.HashTreeRoot(); err != nil {
		return err
	}
	if statetransition.ProcessBlock(post, block) != nil {
		return nil
	}
	if root, err := post.HashTreeRoot(); err == nil {
		block.StateRoot = root
	}
	return nil
}

// Divergences are inputs on which fastssz and typed SSZ are known to differ;
// see zk/stf/tests/vectors.rs.
func Divergences() ([]Input, error) {
	// A header far past a short history: the transition extends
	// JustifiedSlots beyond its SSZ limit, which fastssz hashes anyway.
	pre, err := Genesis(4)
	if err != nil {
		return nil, err
	}
	pre.LatestBlockHeader.Slot = types.HistoricalRootsLimit + 5
	pre.Slot = pre.LatestBlockHeader.Slot
	block, _, err := NextBlock(pre, pre.Slot+1, 0)
	if err != nil {
		return nil, err
	}
	oversize, err := zkstf.NewInput(pre, block)
	if err != nil {
		return nil, err
	}

	// An empty attestation list written as a 4-byte zero offset, which
	// fastssz decodes as an empty list.
	pre, err = Genesis(4)
	if err != nil {
		return nil, err
	}
	block, _, err = NextBlock(pre, 1, 0)
	if err != nil {
		return nil, err
	}
	stateSSZ, err := pre.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	blockSSZ, err := block.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	padded, err := zkstf.EncodeInput(stateSSZ, append(blockSSZ, 0, 0, 0, 0))
	if err != nil {
		return nil, err
	}
	return []Input{
		{Name: "divergence/justified-slots-over-limit", Bytes: oversize},
		{Name: "divergence/empty-list-offset", Bytes: padded},
	}, nil
}
