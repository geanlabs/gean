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
// Each block must apply or be rejected with an error, never panic, and an
// applied block must leave a consistent state.
func FuzzStateTransition(f *testing.F) {
	// Four validators; slot 1 empty, then slot 2 justifies slot 1 from genesis,
	// slot 3 justifies slot 2 from slot 1 and so finalizes slot 1.
	f.Add([]byte{3, 0, 0, 0, 1, 0, 0, 1, 1, 0x0f, 0, 1, 0, 1, 2, 2, 0x0f})

	f.Fuzz(func(t *testing.T, input []byte) {
		in := fuzzInput(input)
		state := makeGenesisState(int(in.next()%8) + 1)

		for range 32 {
			if len(in) == 0 {
				return
			}
			next, err := state.Clone()
			if err != nil {
				t.Fatal(err)
			}
			block, err := buildFuzzBlock(next, &in)
			if err != nil {
				t.Fatalf("build block: %v", err)
			}
			if err := ProcessBlock(next, block); err != nil {
				continue
			}
			checkFuzzState(t, state, next, block)
			state = next
		}
	})
}

// fuzzInput hands out the input a byte at a time, and zeros once it runs out.
type fuzzInput []byte

func (in *fuzzInput) next() byte {
	if len(*in) == 0 {
		return 0
	}
	b := (*in)[0]
	*in = (*in)[1:]
	return b
}

// buildFuzzBlock advances state through up to three skipped slots and builds
// the block for the next one, with up to three attestations. Each names its
// source, target and head by slot and takes that slot's root from the chain the
// block extends; a flag byte corrupts a root or adds voters past the registry,
// and a voter byte picks who votes.
func buildFuzzBlock(state *types.State, in *fuzzInput) (*types.Block, error) {
	numValidators := state.NumValidators()
	slot := state.Slot + 1 + uint64(in.next()%4)
	if err := ProcessSlots(state, slot); err != nil {
		return nil, err
	}
	block := &types.Block{Slot: slot, ProposerIndex: types.ProposerIndex(slot, numValidators), Body: &types.BlockBody{}}
	parentRoot, err := state.LatestBlockHeader.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	block.ParentRoot = parentRoot

	// The history the block's header leaves: the parent, then a zero root per
	// skipped slot.
	history := append(state.HistoricalBlockHashes[:len(state.HistoricalBlockHashes):len(state.HistoricalBlockHashes)], parentRoot[:])
	for range slot - state.LatestBlockHeader.Slot - 1 {
		history = append(history, make([]byte, types.RootSize))
	}

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
		aggregation := types.NewBitlistSSZ(bits)
		for i := range bits {
			if voters>>i&1 != 0 {
				types.BitlistSet(aggregation, i)
			}
		}
		block.Body.Attestations = append(block.Body.Attestations, &types.AggregatedAttestation{AggregationBits: aggregation, Data: data})
	}
	return block, nil
}

func checkFuzzState(t *testing.T, pre, post *types.State, block *types.Block) {
	t.Helper()
	justified, finalized := post.LatestJustified, post.LatestFinalized

	if justified.Slot < pre.LatestJustified.Slot || finalized.Slot < pre.LatestFinalized.Slot {
		t.Fatalf("checkpoints regressed: justified %d -> %d, finalized %d -> %d",
			pre.LatestJustified.Slot, justified.Slot, pre.LatestFinalized.Slot, finalized.Slot)
	}
	if finalized.Slot > justified.Slot {
		t.Fatalf("finalized slot %d is past justified slot %d", finalized.Slot, justified.Slot)
	}
	for _, cp := range []*types.Checkpoint{justified, finalized} {
		if !bytes.Equal(post.HistoricalBlockHashes[cp.Slot], cp.Root[:]) {
			t.Fatalf("checkpoint %d/%x is not on the chain", cp.Slot, cp.Root)
		}
	}
	if ok, err := IsSlotJustified(post, finalized.Slot, justified.Slot); err != nil || !ok {
		t.Fatalf("latest justified slot %d is not marked justified (%v)", justified.Slot, err)
	}
	if got, want := types.BitlistLen(post.JustifiedSlots), block.Slot-1-finalized.Slot; got != want {
		t.Fatalf("%d justified-slot bits past finalized slot %d at slot %d, want %d", got, finalized.Slot, block.Slot, want)
	}
	// Skipped slots leave zero roots in the history votes may name.
	for _, root := range post.JustificationsRoots {
		if types.IsZeroRoot([32]byte(root)) {
			t.Fatalf("zero root among pending justifications")
		}
	}
}
