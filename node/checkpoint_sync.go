package node

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/geanlabs/gean/types"
)

const checkpointSyncTimeout = 30 * time.Second

func downloadCheckpointState(url string) (*types.State, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build checkpoint request: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")

	client := &http.Client{Timeout: checkpointSyncTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download checkpoint state: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checkpoint endpoint returned HTTP %d", resp.StatusCode)
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read checkpoint response: %w", err)
	}

	var state types.State
	if err := state.UnmarshalSSZ(payload); err != nil {
		return nil, fmt.Errorf("decode checkpoint state: %w", err)
	}

	return &state, nil
}

func verifyCheckpointState(state *types.State, genesisTime uint64, genesisValidators []*types.Validator) (*types.State, [32]byte, [32]byte, error) {
	if state == nil {
		return nil, types.ZeroHash, types.ZeroHash, fmt.Errorf("checkpoint state is nil")
	}
	if state.Config == nil {
		return nil, types.ZeroHash, types.ZeroHash, fmt.Errorf("checkpoint state config is nil")
	}
	if state.LatestBlockHeader == nil {
		return nil, types.ZeroHash, types.ZeroHash, fmt.Errorf("checkpoint latest block header is nil")
	}
	if state.LatestJustified == nil {
		return nil, types.ZeroHash, types.ZeroHash, fmt.Errorf("checkpoint latest justified checkpoint is nil")
	}
	if state.LatestFinalized == nil {
		return nil, types.ZeroHash, types.ZeroHash, fmt.Errorf("checkpoint latest finalized checkpoint is nil")
	}
	if state.Config.GenesisTime != genesisTime {
		return nil, types.ZeroHash, types.ZeroHash, fmt.Errorf("genesis time mismatch: expected %d, got %d", genesisTime, state.Config.GenesisTime)
	}
	if len(state.Validators) == 0 {
		return nil, types.ZeroHash, types.ZeroHash, fmt.Errorf("checkpoint state has no validators")
	}
	if len(state.Validators) != len(genesisValidators) {
		return nil, types.ZeroHash, types.ZeroHash, fmt.Errorf("validator count mismatch: expected %d, got %d", len(genesisValidators), len(state.Validators))
	}

	for i := range genesisValidators {
		if genesisValidators[i] == nil {
			return nil, types.ZeroHash, types.ZeroHash, fmt.Errorf("genesis validator %d is nil", i)
		}
		if state.Validators[i] == nil {
			return nil, types.ZeroHash, types.ZeroHash, fmt.Errorf("checkpoint validator %d is nil", i)
		}
		if state.Validators[i].Pubkey != genesisValidators[i].Pubkey {
			return nil, types.ZeroHash, types.ZeroHash, fmt.Errorf("validator pubkey mismatch at index %d", i)
		}
	}

	preparedState := state.Copy()
	originalStateRoot := preparedState.LatestBlockHeader.StateRoot
	preparedState.LatestBlockHeader.StateRoot = types.ZeroHash

	stateRoot, err := preparedState.HashTreeRoot()
	if err != nil {
		return nil, types.ZeroHash, types.ZeroHash, fmt.Errorf("hash checkpoint state: %w", err)
	}
	if originalStateRoot != types.ZeroHash && originalStateRoot != stateRoot {
		return nil, types.ZeroHash, types.ZeroHash, fmt.Errorf("checkpoint header state root mismatch")
	}

	preparedState.LatestBlockHeader.StateRoot = stateRoot
	blockRoot, err := preparedState.LatestBlockHeader.HashTreeRoot()
	if err != nil {
		return nil, types.ZeroHash, types.ZeroHash, fmt.Errorf("hash checkpoint block header: %w", err)
	}
	if err := verifyCheckpointHistory(preparedState, blockRoot); err != nil {
		return nil, types.ZeroHash, types.ZeroHash, err
	}

	return preparedState, stateRoot, blockRoot, nil
}

func verifyCheckpointHistory(state *types.State, anchorRoot [32]byte) error {
	if state.LatestBlockHeader.Slot > state.Slot {
		return fmt.Errorf("checkpoint latest block header slot %d exceeds state slot %d", state.LatestBlockHeader.Slot, state.Slot)
	}
	if state.LatestBlockHeader.Slot > 0 {
		parentSlot, ok := checkpointRootSlot(state, anchorRoot, state.LatestBlockHeader.ParentRoot)
		if !ok {
			return fmt.Errorf("checkpoint parent root not found in canonical history")
		}
		if parentSlot >= state.LatestBlockHeader.Slot {
			return fmt.Errorf("checkpoint parent slot %d is invalid for header slot %d", parentSlot, state.LatestBlockHeader.Slot)
		}
	}
	if err := verifyCheckpointRootAtSlot(state, anchorRoot, state.LatestJustified, "justified"); err != nil {
		return err
	}
	if err := verifyCheckpointRootAtSlot(state, anchorRoot, state.LatestFinalized, "finalized"); err != nil {
		return err
	}
	return nil
}

func verifyCheckpointRootAtSlot(state *types.State, anchorRoot [32]byte, checkpoint *types.Checkpoint, label string) error {
	if checkpoint == nil || checkpoint.Root == types.ZeroHash {
		return nil
	}
	slot, ok := checkpointRootSlot(state, anchorRoot, checkpoint.Root)
	if !ok {
		return fmt.Errorf("checkpoint %s root not found in canonical history", label)
	}
	if slot != checkpoint.Slot {
		return fmt.Errorf("checkpoint %s slot mismatch: expected %d, got %d", label, checkpoint.Slot, slot)
	}
	return nil
}

func checkpointRootSlot(state *types.State, anchorRoot, root [32]byte) (uint64, bool) {
	if root == types.ZeroHash || state == nil || state.LatestBlockHeader == nil {
		return 0, false
	}
	if root == anchorRoot {
		return state.LatestBlockHeader.Slot, true
	}
	for slot, historicalRoot := range state.HistoricalBlockHashes {
		if historicalRoot == root {
			return uint64(slot), true
		}
	}
	return 0, false
}
