package forkchoice

import (
	"maps"
	"slices"
	"testing"

	"github.com/geanlabs/gean/internal/lean/types"
)

// FuzzForkChoiceMatchesSpec drives the proto-array fork choice and the leanSpec
// oracle with the same history and requires the same head after every update.
// The input builds a block tree and delivers it in any order; as in leanSpec, a
// block whose parent fork choice does not hold is not added. It moves
// validators' votes, asks for the head and the safe target from any block, and
// prunes at any of them.
func FuzzForkChoiceMatchesSpec(f *testing.F) {
	f.Add(forkChoiceSeed)

	f.Fuzz(func(t *testing.T, input []byte) {
		in := fuzzInput(input)
		anchor := root(1)
		// roots lists every block parents first, so it gives held blocks a
		// reproducible order.
		roots := [][32]byte{anchor}
		info := map[[32]byte]BlockInfo{anchor: {}}
		for i := range int(in.next()%24) + 1 {
			parent := roots[int(in.next())%len(roots)]
			r := [32]byte{in.next(), byte(i + 1), 1}
			info[r] = BlockInfo{Slot: info[parent].Slot + 1 + uint64(in.next()%3), ParentRoot: parent}
			roots = append(roots, r)
		}
		undelivered := slices.Clone(roots[1:])

		fc := New(0, anchor, [32]byte{})
		// The oracle's view: the blocks fork choice holds and their votes. The
		// oracle gives no weight to a vote whose block is not held.
		held := map[[32]byte]BlockInfo{anchor: {}}
		known := make(map[uint64]*types.AttestationData)
		heldBlock := func(pick byte) [32]byte {
			var blocks [][32]byte
			for _, r := range roots {
				if _, ok := held[r]; ok {
					blocks = append(blocks, r)
				}
			}
			return blocks[int(pick)%len(blocks)]
		}

		for range 256 {
			if len(in) == 0 {
				return
			}
			switch in.next() % 5 {
			case 0:
				if len(undelivered) == 0 {
					continue
				}
				i := int(in.next()) % len(undelivered)
				r := undelivered[i]
				undelivered = slices.Delete(undelivered, i, i+1)
				_, parentHeld := held[info[r].ParentRoot]
				fc.OnBlock(info[r].Slot, r, info[r].ParentRoot)
				if added := fc.NodeIndex(r) >= 0; added != parentHeld {
					t.Fatalf("block %x added=%v, parent held=%v", r[:3], added, parentHeld)
				}
				if parentHeld {
					held[r] = info[r]
				}
			case 1:
				validator, head := uint64(in.next()%8), roots[int(in.next())%len(roots)]
				data := makeAttData(head, info[head].Slot)
				_, isHeld := held[head]
				if accepted := fc.SetKnownVote(validator, head, data.Slot, data); accepted != isHeld {
					t.Fatalf("vote for %x accepted=%v, block held=%v", head[:3], accepted, isHeld)
				}
				if isHeld {
					known[validator] = data
				}
			case 2:
				justified := heldBlock(in.next())
				want, _, err := SpecComputeLMDGhostHead(justified, held, known, 0)
				if err != nil {
					t.Fatal(err)
				}
				if got := fc.UpdateHead(justified); got != want {
					t.Fatalf("head from %x: proto-array %x, spec %x", justified[:3], got[:3], want[:3])
				}
			case 3:
				justified := heldBlock(in.next())
				numValidators, voters := uint64(in.next()%8)+1, in.next()
				votes := make(map[uint64]*types.AttestationData)
				for v := range uint64(8) {
					if voters>>v&1 != 0 {
						head := roots[int(in.next())%len(roots)]
						votes[v] = makeAttData(head, info[head].Slot)
					}
				}
				want, _, err := SpecComputeLMDGhostHead(justified, held, votes, uint64(quorumScore(numValidators)))
				if err != nil {
					t.Fatal(err)
				}
				if got := fc.UpdateSafeTarget(justified, numValidators, votes); got != want {
					t.Fatalf("safe target from %x with %d validators: proto-array %x, spec %x", justified[:3], numValidators, got[:3], want[:3])
				}
			case 4:
				finalized := heldBlock(in.next())
				if finalized == anchor {
					continue
				}
				fc.Prune(finalized)
				anchor = finalized
				kept := map[[32]byte]bool{finalized: true}
				for _, r := range roots {
					if _, ok := held[r]; ok && kept[info[r].ParentRoot] {
						kept[r] = true
					}
				}
				maps.DeleteFunc(held, func(r [32]byte, _ BlockInfo) bool { return !kept[r] })
				if fc.Len() != len(held) {
					t.Fatalf("pruned to %d blocks, spec keeps %d", fc.Len(), len(held))
				}
			}
		}
	})
}

// forkChoiceSeed builds a line A-b0-b1-b2 with a fork f off A. b2 arrives
// first and is dropped, and a vote for it is rejected. Votes on b1 and f put
// the head on f; a safe target over three validators lands on b1; pruning at
// b0 drops f, and the head moves to b1.
var forkChoiceSeed = []byte{
	3,       // four blocks:
	0, 0, 0, // b0 under A
	1, 0, 0, // b1 under b0
	2, 0, 0, // b2 under b1
	0, 0, 0, // f under A
	0, 2, // deliver b2: dropped, b1 is not held
	1, 0, 3, // a vote for b2: rejected
	0, 0, 0, 0, 0, 0, // deliver b0, b1 and f
	1, 0, 2, 1, 1, 4, 1, 2, 4, // votes: v0 on b1, v1 and v2 on f
	2, 0, // head from A: f
	3, 0, 2, 0x07, 2, 2, 2, // safe target from A, three validators all on b1
	4, 1, // prune at b0
	2, 0, // head from b0: b1
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
