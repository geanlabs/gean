package zkvectors

import (
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

// Suite returns the standard vector set: several honest chains plus reject
// cases derived from them. TestApplyMatchesNativeTransition confirms every
// case's expectation against the native transition.
func Suite() ([]Case, error) {
	configs := []ChainConfig{
		// Two-thirds votes every slot: justification and finalization advance.
		{Name: "full", Validators: 4, Slots: 16},
		// Empty slots between blocks and a 6-of-8 supermajority.
		{Name: "skips", Validators: 8, Slots: 24,
			Skip:         func(s uint64) bool { return s%5 == 3 || s%7 == 0 },
			Participants: func(uint64) int { return 6 }},
		// Votes below two thirds: justifications accumulate but never land.
		{Name: "minority", Validators: 6, Slots: 10,
			Participants: func(uint64) int { return 3 }},
		// Votes at exactly two thirds, the supermajority boundary.
		{Name: "two-thirds", Validators: 6, Slots: 8,
			Participants: func(uint64) int { return 4 }},
		// Blocks without votes.
		{Name: "empty", Validators: 3, Slots: 6,
			Participants: func(uint64) int { return 0 }},
		// The full registry, to exercise state size.
		{Name: "registry-4096", Validators: int(types.ValidatorRegistryLimit), Slots: 4},
	}

	var cases []Case
	for _, cfg := range configs {
		chain, err := Chain(cfg)
		if err != nil {
			return nil, err
		}
		cases = append(cases, chain...)
	}

	base, err := Chain(configs[0])
	if err != nil {
		return nil, err
	}
	rejects, err := Rejects(base[len(base)/2])
	if err != nil {
		return nil, err
	}
	return append(cases, rejects...), nil
}

// Rejects derives invalid blocks from a valid case, one fault each.
func Rejects(valid Case) ([]Case, error) {
	n := valid.Pre.NumValidators()
	faults := []struct {
		name   string
		mutate func(b *types.Block)
	}{
		{"bad-state-root", func(b *types.Block) { b.StateRoot[0] ^= 1 }},
		{"bad-parent-root", func(b *types.Block) { b.ParentRoot[0] ^= 1 }},
		{"wrong-proposer", func(b *types.Block) { b.ProposerIndex = (b.ProposerIndex + 1) % n }},
		{"stale-slot", func(b *types.Block) { b.Slot = valid.Pre.Slot }},
		{"too-many-attestation-data", func(b *types.Block) {
			b.Body.Attestations = nil
			for i := range types.MaxAttestationsData + 1 {
				b.Body.Attestations = append(b.Body.Attestations, &types.AggregatedAttestation{
					AggregationBits: types.BitlistFromIndices([]uint64{0}),
					Data: &types.AttestationData{
						Slot:   uint64(i),
						Head:   &types.Checkpoint{},
						Target: &types.Checkpoint{},
						Source: &types.Checkpoint{},
					},
				})
			}
		}},
		{"voter-out-of-range", func(b *types.Block) {
			if len(b.Body.Attestations) > 0 {
				b.Body.Attestations[0].AggregationBits = types.BitlistFromIndices([]uint64{n})
			}
		}},
	}
	if len(valid.Block.Body.Attestations) == 0 {
		return nil, fmt.Errorf("reject base %s carries no vote", valid.Name)
	}

	out := make([]Case, 0, len(faults))
	for _, f := range faults {
		block, err := cloneBlock(valid.Block)
		if err != nil {
			return nil, err
		}
		f.mutate(block)
		out = append(out, Case{Name: "reject/" + f.name, Pre: valid.Pre, Block: block, WantErr: true})
	}
	return out, nil
}

func cloneBlock(b *types.Block) (*types.Block, error) {
	enc, err := b.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	out := new(types.Block)
	return out, out.UnmarshalSSZ(enc)
}
