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

	return preparedState, stateRoot, blockRoot, nil
}
