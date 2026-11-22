package specfixtures

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/geanlabs/gean/internal/lean/types"
)

func (ts *TestState) ToState() (*types.State, error) {
	if ts == nil {
		return nil, fmt.Errorf("state fixture is nil")
	}
	parentRoot, err := ParseHexRoot(ts.LatestBlockHeader.ParentRoot)
	if err != nil {
		return nil, fmt.Errorf("latestBlockHeader.parentRoot: %w", err)
	}
	stateRoot, err := ParseHexRoot(ts.LatestBlockHeader.StateRoot)
	if err != nil {
		return nil, fmt.Errorf("latestBlockHeader.stateRoot: %w", err)
	}
	bodyRoot, err := ParseHexRoot(ts.LatestBlockHeader.BodyRoot)
	if err != nil {
		return nil, fmt.Errorf("latestBlockHeader.bodyRoot: %w", err)
	}
	justifiedRoot, err := ParseHexRoot(ts.LatestJustified.Root)
	if err != nil {
		return nil, fmt.Errorf("latestJustified.root: %w", err)
	}
	finalizedRoot, err := ParseHexRoot(ts.LatestFinalized.Root)
	if err != nil {
		return nil, fmt.Errorf("latestFinalized.root: %w", err)
	}

	state := &types.State{
		Config: &types.ChainConfig{GenesisTime: ts.Config.GenesisTime},
		Slot:   ts.Slot,
		LatestBlockHeader: &types.BlockHeader{
			Slot:          ts.LatestBlockHeader.Slot,
			ProposerIndex: ts.LatestBlockHeader.ProposerIndex,
			ParentRoot:    parentRoot,
			StateRoot:     stateRoot,
			BodyRoot:      bodyRoot,
		},
		LatestJustified: &types.Checkpoint{Root: justifiedRoot, Slot: ts.LatestJustified.Slot},
		LatestFinalized: &types.Checkpoint{Root: finalizedRoot, Slot: ts.LatestFinalized.Slot},
	}

	for i, v := range ts.Validators.Data {
		if v.AttestationPubkey == "" {
			return nil, fmt.Errorf("validators[%d].attestationPubkey: missing", i)
		}
		if v.ProposalPubkey == "" {
			return nil, fmt.Errorf("validators[%d].proposalPubkey: missing", i)
		}
		attPk, err := ParseStateKey(v.AttestationPubkey)
		if err != nil {
			return nil, fmt.Errorf("validators[%d].attestationPubkey: %w", i, err)
		}
		propPk, err := ParseStateKey(v.ProposalPubkey)
		if err != nil {
			return nil, fmt.Errorf("validators[%d].proposalPubkey: %w", i, err)
		}
		state.Validators = append(state.Validators, &types.Validator{AttestationPubkey: attPk, ProposalPubkey: propPk, Index: v.Index})
	}

	if state.HistoricalBlockHashes, err = parseRootList("historicalBlockHashes", ts.HistoricalBlockHashes.Data); err != nil {
		return nil, err
	}
	if state.JustifiedSlots, err = ParseBoolBitlist(ts.JustifiedSlots.Data); err != nil {
		return nil, fmt.Errorf("justifiedSlots: %w", err)
	}
	if state.JustificationsRoots, err = parseRootList("justificationsRoots", ts.JustificationsRoots.Data); err != nil {
		return nil, err
	}
	if state.JustificationsValidators, err = ParseBoolBitlist(ts.JustificationsValidators.Data); err != nil {
		return nil, fmt.Errorf("justificationsValidators: %w", err)
	}

	return state, nil
}

func parseRootList(field string, data []json.RawMessage) ([][]byte, error) {
	out := make([][]byte, 0, len(data))
	for i, raw := range data {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", field, i, err)
		}
		root, err := ParseHexRoot(s)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", field, i, err)
		}
		h := make([]byte, types.RootSize)
		copy(h, root[:])
		out = append(out, h)
	}
	return out, nil
}

// PoseidonPublicKeySize is the size of leanSpec's Poseidon XMSS public keys,
// which its fixtures carry until they ship leanVM's BLAKE2s keys (leanSpec#1210).
const PoseidonPublicKeySize = 52

// ParseStateKey parses a validator key for a state. A state hashes each key by
// its hash tree root, and a 32-byte key's root is the key itself, so a Poseidon
// key's root stands in for it and every state and block root stays equal to
// leanSpec's. No signature can be checked against it.
func ParseStateKey(s string) ([types.PubkeySize]byte, error) {
	if b, err := ParseHexBytes(s); err == nil && len(b) == PoseidonPublicKeySize {
		var chunks [64]byte
		copy(chunks[:], b)
		return sha256.Sum256(chunks[:]), nil
	}
	return ParseHexPubkey(s)
}
