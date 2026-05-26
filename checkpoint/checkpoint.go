package checkpoint

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/geanlabs/gean/types"
)

// Timeouts
const (
	CheckpointConnectTimeout = 15 * time.Second
	CheckpointReadTimeout    = 15 * time.Second
)

// Endpoint paths defined by leanSpec.
const (
	StatesFinalizedPath = "/lean/v0/states/finalized"
	BlocksFinalizedPath = "/lean/v0/blocks/finalized"
)

// FetchCheckpointState downloads and verifies a finalized state from a peer.
func FetchCheckpointState(
	url string,
	expectedGenesisTime uint64,
	expectedValidators []*types.Validator,
) (*types.State, error) {
	body, err := fetchSSZ(url)
	if err != nil {
		return nil, err
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

// FetchCheckpointAnchor downloads the finalized state AND the matching
// finalized SignedBlock, verifies the state↔block pair, runs the same
// VerifyCheckpointState rules as FetchCheckpointState, and returns both.
//
// The stateURL is expected to end in StatesFinalizedPath; the block URL is
// derived by replacing that suffix with BlocksFinalizedPath. Per leanSpec
// PR #713 the pair is bound by block.state_root == hash_tree_root(state) —
// a synthetic block fabricated from state.latest_block_header (zero
// signature, empty body) will fail this check whenever the real anchor body
// carried attestations, which is the typical post-genesis case.
func FetchCheckpointAnchor(
	stateURL string,
	expectedGenesisTime uint64,
	expectedValidators []*types.Validator,
) (*types.State, *types.SignedBlock, error) {
	blockURL, err := deriveBlockURL(stateURL)
	if err != nil {
		return nil, nil, err
	}

	stateBody, err := fetchSSZ(stateURL)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch state: %w", err)
	}
	state := &types.State{}
	if err := state.UnmarshalSSZ(stateBody); err != nil {
		return nil, nil, fmt.Errorf("state ssz decode: %w", err)
	}

	blockBody, err := fetchSSZ(blockURL)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch block: %w", err)
	}
	signedBlock := &types.SignedBlock{}
	if err := signedBlock.UnmarshalSSZ(blockBody); err != nil {
		return nil, nil, fmt.Errorf("block ssz decode: %w", err)
	}
	if signedBlock.Block == nil {
		return nil, nil, fmt.Errorf("block ssz decode: nil Block")
	}

	stateRoot, err := state.HashTreeRoot()
	if err != nil {
		return nil, nil, fmt.Errorf("state hash_tree_root: %w", err)
	}
	if signedBlock.Block.StateRoot != stateRoot {
		return nil, nil, fmt.Errorf(
			"checkpoint state/block pair mismatch: block.state_root=0x%x, hash_tree_root(state)=0x%x",
			signedBlock.Block.StateRoot, stateRoot,
		)
	}

	if err := VerifyCheckpointState(state, expectedGenesisTime, expectedValidators); err != nil {
		return nil, nil, fmt.Errorf("verify state: %w", err)
	}

	return state, signedBlock, nil
}

// deriveBlockURL converts a /lean/v0/states/finalized URL into the matching
// /lean/v0/blocks/finalized URL, preserving scheme, host, port, and query.
func deriveBlockURL(stateURL string) (string, error) {
	if !strings.Contains(stateURL, StatesFinalizedPath) {
		return "", fmt.Errorf(
			"checkpoint URL %q does not contain %q; expected the finalized-state endpoint so the block endpoint can be derived",
			stateURL, StatesFinalizedPath,
		)
	}
	return strings.Replace(stateURL, StatesFinalizedPath, BlocksFinalizedPath, 1), nil
}

// fetchSSZ performs the shared HTTP GET + body read used by both
// FetchCheckpointState and FetchCheckpointAnchor.
func fetchSSZ(url string) ([]byte, error) {
	client := &http.Client{Timeout: CheckpointConnectTimeout + CheckpointReadTimeout}

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
		return nil, fmt.Errorf("http status %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return body, nil
}

// VerifyCheckpointState runs all 12 validation checks.
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
		if v.AttestationPubkey != expectedValidators[i].AttestationPubkey {
			return fmt.Errorf("validator %d attestation pubkey mismatch", i)
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
