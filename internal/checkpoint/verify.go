package checkpoint

import (
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

func VerifyCheckpointState(
	state *types.State,
	expectedGenesisTime uint64,
	expectedValidators []*types.Validator,
) error {
	if err := validateStateShape(state); err != nil {
		return err
	}
	if state.Slot == 0 {
		return fmt.Errorf("checkpoint state slot cannot be 0")
	}
	if len(state.Validators) == 0 {
		return fmt.Errorf("checkpoint state has no validators")
	}
	if state.Config.GenesisTime != expectedGenesisTime {
		return fmt.Errorf("genesis time mismatch: expected %d, got %d",
			expectedGenesisTime, state.Config.GenesisTime)
	}
	if len(state.Validators) != len(expectedValidators) {
		return fmt.Errorf("validator count mismatch: expected %d, got %d",
			len(expectedValidators), len(state.Validators))
	}

	for i, v := range state.Validators {
		if v == nil {
			return fmt.Errorf("validator %d is nil", i)
		}
		if v.Index != uint64(i) {
			return fmt.Errorf("validator at position %d has non-sequential index: expected %d, got %d",
				i, i, v.Index)
		}
	}

	if _, err := state.HashTreeRoot(); err != nil {
		return fmt.Errorf("checkpoint state hash_tree_root: %w", err)
	}

	for i, v := range state.Validators {
		if expectedValidators[i] == nil {
			return fmt.Errorf("expected validator %d is nil", i)
		}
		if v.AttestationPubkey != expectedValidators[i].AttestationPubkey {
			return fmt.Errorf("validator %d attestation pubkey mismatch", i)
		}
		if v.ProposalPubkey != expectedValidators[i].ProposalPubkey {
			return fmt.Errorf("validator %d proposal pubkey mismatch", i)
		}
	}

	if state.LatestFinalized.Slot > state.Slot {
		return fmt.Errorf("finalized slot %d exceeds state slot %d",
			state.LatestFinalized.Slot, state.Slot)
	}
	if state.LatestJustified.Slot > state.Slot {
		return fmt.Errorf("justified slot %d exceeds state slot %d",
			state.LatestJustified.Slot, state.Slot)
	}
	if state.LatestJustified.Slot < state.LatestFinalized.Slot {
		return fmt.Errorf("justified slot %d precedes finalized slot %d",
			state.LatestJustified.Slot, state.LatestFinalized.Slot)
	}
	if state.LatestJustified.Slot == state.LatestFinalized.Slot &&
		state.LatestJustified.Root != state.LatestFinalized.Root {
		return fmt.Errorf("justified and finalized at same slot %d have different roots",
			state.LatestJustified.Slot)
	}
	if state.LatestBlockHeader.Slot > state.Slot {
		return fmt.Errorf("block header slot %d exceeds state slot %d",
			state.LatestBlockHeader.Slot, state.Slot)
	}

	blockRoot, err := state.LatestBlockHeader.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("block header hash_tree_root: %w", err)
	}
	if state.LatestBlockHeader.Slot == state.LatestFinalized.Slot &&
		blockRoot != state.LatestFinalized.Root {
		return fmt.Errorf("block header at finalized slot %d has mismatched root",
			state.LatestFinalized.Slot)
	}
	if state.LatestBlockHeader.Slot == state.LatestJustified.Slot &&
		blockRoot != state.LatestJustified.Root {
		return fmt.Errorf("block header at justified slot %d has mismatched root",
			state.LatestJustified.Slot)
	}

	return nil
}

func validateStateShape(state *types.State) error {
	if state == nil {
		return fmt.Errorf("checkpoint state is nil")
	}
	if state.Config == nil {
		return fmt.Errorf("checkpoint state config is nil")
	}
	if state.LatestBlockHeader == nil {
		return fmt.Errorf("checkpoint state latest block header is nil")
	}
	if state.LatestJustified == nil {
		return fmt.Errorf("checkpoint state latest justified is nil")
	}
	if state.LatestFinalized == nil {
		return fmt.Errorf("checkpoint state latest finalized is nil")
	}
	return nil
}
