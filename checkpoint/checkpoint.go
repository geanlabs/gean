package checkpoint

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/geanlabs/gean/types"
)

// Timeouts matching ethlambda checkpoint_sync.rs L9-13.
const (
	CheckpointConnectTimeout = 15 * time.Second
	CheckpointReadTimeout    = 15 * time.Second
)

// FetchCheckpointState downloads and verifies a finalized state from a peer.
// Matches ethlambda checkpoint_sync.rs fetch_checkpoint_state (L65-90).
func FetchCheckpointState(
	url string,
	expectedGenesisTime uint64,
	expectedValidators []*types.Validator,
) (*types.State, error) {
	client := &http.Client{
		Timeout: CheckpointConnectTimeout + CheckpointReadTimeout,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	state := &types.State{}
	if err := state.UnmarshalSSZ(body); err != nil {
		return nil, fmt.Errorf("ssz decode: %w", err)
	}

	if err := VerifyCheckpointState(state, expectedGenesisTime, expectedValidators); err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}

	return state, nil
}

// VerifyCheckpointState runs all 12 validation checks.
// Matches ethlambda checkpoint_sync.rs verify_checkpoint_state (L98-189).
func VerifyCheckpointState(
	state *types.State,
	expectedGenesisTime uint64,
	expectedValidators []*types.Validator,
) error {
	// 1. Slot != 0
	if state.Slot == 0 {
		return fmt.Errorf("checkpoint state slot cannot be 0")
	}

	// 2. Has validators
	if len(state.Validators) == 0 {
		return fmt.Errorf("checkpoint state has no validators")
	}

	// 3. Genesis time matches
	if state.Config.GenesisTime != expectedGenesisTime {
		return fmt.Errorf("genesis time mismatch: expected %d, got %d",
			expectedGenesisTime, state.Config.GenesisTime)
	}

	// 4. Validator count matches
	if len(state.Validators) != len(expectedValidators) {
		return fmt.Errorf("validator count mismatch: expected %d, got %d",
			len(expectedValidators), len(state.Validators))
	}

	// 5. Validator indices sequential
	for i, v := range state.Validators {
		if v.Index != uint64(i) {
			return fmt.Errorf("validator at position %d has non-sequential index: expected %d, got %d",
				i, i, v.Index)
		}
	}

	// 6. Validator pubkeys match
	for i, v := range state.Validators {
		if v.Pubkey != expectedValidators[i].Pubkey {
			return fmt.Errorf("validator %d pubkey mismatch", i)
		}
	}

	// 7. Finalized slot <= state slot
	if state.LatestFinalized.Slot > state.Slot {
		return fmt.Errorf("finalized slot %d exceeds state slot %d",
			state.LatestFinalized.Slot, state.Slot)
	}

	// 8. Justified slot >= finalized slot
	if state.LatestJustified.Slot < state.LatestFinalized.Slot {
		return fmt.Errorf("justified slot %d precedes finalized slot %d",
			state.LatestJustified.Slot, state.LatestFinalized.Slot)
	}

	// 9. If justified == finalized slot, roots must match
	if state.LatestJustified.Slot == state.LatestFinalized.Slot &&
		state.LatestJustified.Root != state.LatestFinalized.Root {
		return fmt.Errorf("justified and finalized at same slot %d have different roots",
			state.LatestJustified.Slot)
	}

	// 10. Block header slot <= state slot
	if state.LatestBlockHeader.Slot > state.Slot {
		return fmt.Errorf("block header slot %d exceeds state slot %d",
			state.LatestBlockHeader.Slot, state.Slot)
	}

	// 11. If block header slot == finalized slot, roots must match
	blockRoot, _ := state.LatestBlockHeader.HashTreeRoot()
	if state.LatestBlockHeader.Slot == state.LatestFinalized.Slot &&
		blockRoot != state.LatestFinalized.Root {
		return fmt.Errorf("block header at finalized slot %d has mismatched root",
			state.LatestFinalized.Slot)
	}

	// 12. If block header slot == justified slot, roots must match
	if state.LatestBlockHeader.Slot == state.LatestJustified.Slot &&
		blockRoot != state.LatestJustified.Root {
		return fmt.Errorf("block header at justified slot %d has mismatched root",
			state.LatestJustified.Slot)
	}

	return nil
}
