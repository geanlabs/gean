//go:build spectests

package lean

import (
	"encoding/binary"
	"testing"

	"github.com/geanlabs/gean/internal/lean/specfixtures"
	"github.com/geanlabs/gean/internal/lean/types"
)

// syncFixture covers the spec's verify_checkpoint_state: an SSZ-encoded state
// and whether its structure is acceptable as a checkpoint.
type syncFixture struct {
	specfixtures.Header
	Operation struct {
		Kind          string `json:"kind"`
		NumValidators uint64 `json:"numValidators"`
		AnchorSlot    uint64 `json:"anchorSlot"`
	} `json:"operation"`
	Output struct {
		Valid          bool   `json:"valid"`
		StateBytes     string `json:"stateBytes"`
		ValidatorCount uint64 `json:"validatorCount"`
		AnchorSlot     uint64 `json:"anchorSlot"`
	} `json:"output"`
}

func runSync(t *testing.T, fixture *syncFixture) {
	if fixture.Operation.Kind != "verify_checkpoint" {
		t.Fatalf("unknown sync operation %q", fixture.Operation.Kind)
	}

	stateBytes := mustParse(t, specfixtures.ParseHexBytes, fixture.Output.StateBytes)
	state := &types.State{}
	err := state.UnmarshalSSZ(stateBytes)
	// The state ships as raw SSZ, so no key shows up as hex for the shared
	// check: a state that only decodes at Poseidon validator sizes is one.
	if err != nil && poseidonValidatorList(stateBytes, fixture.Output.ValidatorCount) {
		skip(t, skipPoseidonXMSS, "state holds Poseidon validator keys")
	}
	if err != nil {
		if fixture.Output.Valid {
			t.Fatalf("decode a valid checkpoint state: %v", err)
		}
		return
	}

	if got := verifyCheckpointStructure(state); got != fixture.Output.Valid {
		t.Errorf("checkpoint state accepted = %v, want %v (validators=%d slot=%d)",
			got, fixture.Output.Valid, len(state.Validators), state.Slot)
	}
	if uint64(len(state.Validators)) != fixture.Output.ValidatorCount {
		t.Errorf("validator count %d, want %d", len(state.Validators), fixture.Output.ValidatorCount)
	}
	if state.Slot != fixture.Output.AnchorSlot {
		t.Errorf("anchor slot %d, want %d", state.Slot, fixture.Output.AnchorSlot)
	}
}

// verifyCheckpointStructure is the spec's verify_checkpoint_state: at least
// one validator, no more than the registry limit, and a computable root.
// gean's checkpoint.VerifyCheckpointState is stricter: it also checks the
// genesis time and validator set against the node's config and rejects slot
// 0, so the narrower spec contract is checked here.
func verifyCheckpointStructure(state *types.State) bool {
	if len(state.Validators) == 0 || uint64(len(state.Validators)) > types.ValidatorRegistryLimit {
		return false
	}
	_, err := state.HashTreeRoot()
	return err == nil
}

// poseidonValidatorList reports whether an SSZ state's validator list is sized
// for Poseidon keys: 112 bytes a validator (two 52-byte keys and an index)
// where BLAKE2s keys make it 72.
func poseidonValidatorList(state []byte, validatorCount uint64) bool {
	// The state's fixed fields take 208 bytes; the offsets of its variable
	// fields follow, the validator list's third, then the field after it.
	const validatorsOffset, nextFieldOffset = 216, 220
	if validatorCount == 0 || len(state) < nextFieldOffset+4 {
		return false
	}
	start := binary.LittleEndian.Uint32(state[validatorsOffset:])
	end := binary.LittleEndian.Uint32(state[nextFieldOffset:])
	return end >= start && uint64(end-start) == validatorCount*(2*poseidonPublicKeySize+8)
}
