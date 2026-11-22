package transition

import (
	"bytes"
	"testing"

	"github.com/geanlabs/gean/internal/lean/types"
)

// FuzzStateTransition drives a chain of blocks built from the input. The
// harness fills in each block's proposer and parent root, so blocks pass the
// header checks and the fuzzer explores attestation processing, justification
// and finality; the input picks the skipped slots, the votes' checkpoints, which
// validators vote, and corrupted roots and voter bits. Raw mutated blocks almost
// never get past the parent-root check.
//
// Each block must apply or be rejected with an error, never panic. An applied
// block must leave a consistent state, and replaying it through StateTransition
// with its state root must reproduce that state.
func FuzzStateTransition(f *testing.F) {
	// Four validators; slot 1 empty, then slot 2 justifies slot 1 from genesis,
	// slot 3 justifies slot 2 from slot 1 and so finalizes slot 1.
	f.Add([]byte{3, 0, 0, 0, 1, 0, 0, 1, 1, 0x0f, 0, 1, 0, 1, 2, 2, 0x0f})
	f.Add([]byte{7, 3, 2, 0, 1, 2, 0, 0xff, 1, 2, 3, 3, 0x02, 0x7f})

	f.Fuzz(func(t *testing.T, input []byte) {
		in := fuzzInput(input)
		numValidators := uint64(in.next()%8) + 1
		state := makeGenesisState(int(numValidators))

		for range 32 {
			if in.empty() {
				return
			}
			block, err := buildFuzzBlock(state, numValidators, &in)
			if err != nil {
				t.Fatalf("build block: %v", err)
			}
			next, err := state.Clone()
			if err != nil {
				t.Fatal(err)
			}
			if err := ProcessSlots(next, block.Slot); err != nil {
				t.Fatalf("process slots to %d: %v", block.Slot, err)
			}
			if err := ProcessBlock(next, block); err != nil {
				continue
			}
			checkFuzzState(t, state, next, block, numValidators)

			root, err := next.HashTreeRoot()
			if err != nil {
				t.Fatal(err)
			}
			block.StateRoot = root
			replay, err := state.Clone()
			if err != nil {
				t.Fatal(err)
			}
			if err := StateTransition(replay, block); err != nil {
				t.Fatalf("replaying an applied block at slot %d: %v", block.Slot, err)
			}
			state = next
		}
	})
}

// fuzzInput hands out the input a byte at a time, and zeros once it runs out.
type fuzzInput []byte

func (in *fuzzInput) empty() bool { return len(*in) == 0 }

func (in *fuzzInput) next() byte {
	if len(*in) == 0 {
		return 0
	}
	b := (*in)[0]
	*in = (*in)[1:]
	return b
}

// buildFuzzBlock builds the block after state: up to three skipped slots, then
// up to three attestations. Each names its source, target and head by slot and
// takes that slot's root from the chain the block extends; a flag byte corrupts
// a root or adds voters past the registry, and a voter byte picks who votes.
func buildFuzzBlock(state *types.State, numValidators uint64, in *fuzzInput) (*types.Block, error) {
	slot := state.Slot + 1 + uint64(in.next()%4)
	block := &types.Block{Slot: slot, ProposerIndex: types.ProposerIndex(slot, numValidators), Body: &types.BlockBody{}}

	// The chain the block extends: a header-only copy of it gives the parent root
	// and the historical roots its votes may name.
	chain, err := state.Clone()
	if err != nil {
		return nil, err
	}
	if err := ProcessSlots(chain, slot); err != nil {
		return nil, err
	}
	if block.ParentRoot, err = chain.LatestBlockHeader.HashTreeRoot(); err != nil {
		return nil, err
	}
	if err := ProcessBlockHeader(chain, block); err != nil {
		return nil, err
	}
	history := chain.HistoricalBlockHashes

	checkpoint := func(flags byte) *types.Checkpoint {
		cp := &types.Checkpoint{Slot: uint64(in.next()) % uint64(len(history))}
		copy(cp.Root[:], history[cp.Slot])
		if flags&1 != 0 {
			cp.Root[0] ^= 0xff
		}
		return cp
	}
	for range in.next() % 4 {
		flags := in.next()
		data := &types.AttestationData{Slot: slot - 1, Source: checkpoint(flags), Target: checkpoint(flags >> 1), Head: checkpoint(flags >> 2)}
		voters, bits := in.next(), numValidators
		if flags&8 != 0 {
			bits = 8
		}
		var indices []uint64
		for i := range bits {
			if voters&(1<<i) != 0 {
				indices = append(indices, i)
			}
		}
		aggregation := types.NewBitlistSSZ(bits)
		for _, i := range indices {
			types.BitlistSet(aggregation, i)
		}
		block.Body.Attestations = append(block.Body.Attestations, &types.AggregatedAttestation{AggregationBits: aggregation, Data: data})
	}
	return block, nil
}

// checkFuzzState checks the state an applied block left against the state
// before it.
func checkFuzzState(t *testing.T, pre, post *types.State, block *types.Block, numValidators uint64) {
	t.Helper()
	justified, finalized := post.LatestJustified, post.LatestFinalized

	if post.Slot != block.Slot || post.LatestBlockHeader.Slot != block.Slot {
		t.Fatalf("state slot %d, header slot %d, block slot %d", post.Slot, post.LatestBlockHeader.Slot, block.Slot)
	}
	if uint64(len(post.HistoricalBlockHashes)) != block.Slot {
		t.Fatalf("%d historical roots at slot %d", len(post.HistoricalBlockHashes), block.Slot)
	}
	if justified.Slot < pre.LatestJustified.Slot || finalized.Slot < pre.LatestFinalized.Slot {
		t.Fatalf("checkpoints regressed: justified %d -> %d, finalized %d -> %d",
			pre.LatestJustified.Slot, justified.Slot, pre.LatestFinalized.Slot, finalized.Slot)
	}
	if finalized.Slot > justified.Slot {
		t.Fatalf("finalized slot %d is past justified slot %d", finalized.Slot, justified.Slot)
	}
	for name, cp := range map[string]*types.Checkpoint{"justified": justified, "finalized": finalized} {
		if !bytes.Equal(post.HistoricalBlockHashes[cp.Slot], cp.Root[:]) {
			t.Fatalf("%s checkpoint at slot %d is not on the chain", name, cp.Slot)
		}
	}
	if ok, err := IsSlotJustified(post, finalized.Slot, justified.Slot); err != nil || !ok {
		t.Fatalf("latest justified slot %d is not marked justified (%v)", justified.Slot, err)
	}
	if got, want := types.BitlistLen(post.JustifiedSlots), block.Slot-1-min(finalized.Slot, block.Slot-1); got != want {
		t.Fatalf("%d justified-slot bits past finalized slot %d at slot %d, want %d", got, finalized.Slot, block.Slot, want)
	}
	if got, want := types.BitlistLen(post.JustificationsValidators), uint64(len(post.JustificationsRoots))*numValidators; got != want {
		t.Fatalf("%d justification vote bits for %d roots, want %d", got, len(post.JustificationsRoots), want)
	}
	for i, root := range post.JustificationsRoots {
		if types.IsZeroRoot([32]byte(root)) {
			t.Fatalf("zero root among pending justifications")
		}
		if i > 0 && bytes.Compare(post.JustificationsRoots[i-1], root) >= 0 {
			t.Fatalf("pending justification roots are not sorted and distinct")
		}
	}
}
