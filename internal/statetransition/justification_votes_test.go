package statetransition

import (
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func TestJustificationVotesReadsOneRow(t *testing.T) {
	const validators = 3
	s := makeGenesisState(validators)
	rootA, rootB := [32]byte{0xa}, [32]byte{0xb}
	s.JustificationsRoots = [][]byte{rootA[:], rootB[:]}
	s.JustificationsValidators = types.NewBitlistSSZ(2 * validators)
	types.BitlistSet(s.JustificationsValidators, 1)            // A: validator 1
	types.BitlistSet(s.JustificationsValidators, validators+0) // B: validator 0
	types.BitlistSet(s.JustificationsValidators, validators+2) // B: validator 2

	for _, tc := range []struct {
		name string
		root [32]byte
		want []bool
	}{
		{"first row", rootA, []bool{false, true, false}},
		{"second row", rootB, []bool{true, false, true}},
		{"untracked", [32]byte{0xc}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := JustificationVotes(s, tc.root)
			if len(got) != len(tc.want) {
				t.Fatalf("votes = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("votes = %v, want %v", got, tc.want)
				}
			}
		})
	}

	// A row count that does not match the vote list is a malformed state.
	s.JustificationsRoots = append(s.JustificationsRoots, make([]byte, types.RootSize))
	if JustificationVotes(s, rootA) != nil {
		t.Fatal("read a row from a malformed state")
	}
	if JustificationVotes(nil, rootA) != nil {
		t.Fatal("read a row from a nil state")
	}
}
