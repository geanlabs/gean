package checkpoint

import (
	"testing"

	"github.com/geanlabs/gean/internal/lean/types"
)

// FuzzCheckpointAnchor checks that verifying any state a checkpoint source could
// serve never panics. A source controls the block too, so it can always pair a
// state with a matching block; the harness only zeroes the header's state root,
// as that pairing requires.
func FuzzCheckpointAnchor(f *testing.F) {
	seed, _ := makeAnchorPair(f)
	expectedValidators := copyValidators(seed.Validators)
	seedSSZ, err := seed.MarshalSSZ()
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seedSSZ)

	f.Fuzz(func(t *testing.T, stateSSZ []byte) {
		state := &types.State{}
		if err := state.UnmarshalSSZ(stateSSZ); err != nil || state.LatestBlockHeader == nil {
			return
		}
		state.LatestBlockHeader.StateRoot = types.ZeroRoot
		_ = VerifyCheckpointState(state, seed.Config.GenesisTime, expectedValidators)
	})
}
