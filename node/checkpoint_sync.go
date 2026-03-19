package node

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/geanlabs/gean/types"
)

// fetchCheckpointState downloads an SSZ-encoded finalized state from a peer's API.
func fetchCheckpointState(url string) (*types.State, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			ResponseHeaderTimeout: 15 * time.Second,
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from checkpoint peer", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	state := new(types.State)
	if err := state.UnmarshalSSZ(body); err != nil {
		return nil, fmt.Errorf("SSZ deserialization failed: %w", err)
	}

	return state, nil
}

// verifyCheckpointState validates a checkpoint state against local genesis config.
func verifyCheckpointState(state *types.State, expectedGenesisTime uint64, expectedValidators []*types.Validator) error {
	// Slot sanity: must not be genesis.
	if state.Slot == 0 {
		return fmt.Errorf("checkpoint state slot cannot be 0")
	}

	// Validators must exist.
	if len(state.Validators) == 0 {
		return fmt.Errorf("checkpoint state has no validators")
	}

	// Genesis time must match local config.
	if state.Config.GenesisTime != expectedGenesisTime {
		return fmt.Errorf("genesis time mismatch: expected %d, got %d", expectedGenesisTime, state.Config.GenesisTime)
	}

	// Validator count must match.
	if len(state.Validators) != len(expectedValidators) {
		return fmt.Errorf("validator count mismatch: expected %d, got %d", len(expectedValidators), len(state.Validators))
	}

	// Validator pubkeys must match (critical security check).
	for i, sv := range state.Validators {
		if sv.Pubkey != expectedValidators[i].Pubkey {
			return fmt.Errorf("validator %d pubkey mismatch", i)
		}
	}

	// Finalized slot must not exceed state slot.
	if state.LatestFinalized.Slot > state.Slot {
		return fmt.Errorf("finalized slot %d exceeds state slot %d", state.LatestFinalized.Slot, state.Slot)
	}

	// Justified must be at or after finalized.
	if state.LatestJustified.Slot < state.LatestFinalized.Slot {
		return fmt.Errorf("justified slot %d precedes finalized slot %d", state.LatestJustified.Slot, state.LatestFinalized.Slot)
	}

	// If justified and finalized at same slot, roots must match.
	if state.LatestJustified.Slot == state.LatestFinalized.Slot && state.LatestJustified.Root != state.LatestFinalized.Root {
		return fmt.Errorf("justified and finalized at same slot %d have mismatched roots", state.LatestJustified.Slot)
	}

	// Block header slot must not exceed state slot.
	if state.LatestBlockHeader.Slot > state.Slot {
		return fmt.Errorf("block header slot %d exceeds state slot %d", state.LatestBlockHeader.Slot, state.Slot)
	}

	return nil
}
