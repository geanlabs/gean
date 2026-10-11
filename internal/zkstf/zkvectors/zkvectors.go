// Package zkvectors generates (pre-state, block) cases for checking a zkVM
// guest against the native state transition.
//
// The leanSpec state-transition fixtures cannot serve here yet: they encode
// 52-byte XMSS keys while gean's types carry 32-byte keys, so their roots do
// not reproduce. These chains are built with gean's own transition instead.
// They exercise the same machinery (empty slots, votes, justification,
// finalization, the distinct-data cap, large registries), and
// TestApplyMatchesNativeTransition checks every case natively. Spec conformance stays with the
// spectests; these vectors establish guest/native equivalence.
package zkvectors

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/types"
)

// Case is one block applied to one pre-state.
type Case struct {
	Name    string
	Pre     *types.State
	Block   *types.Block
	WantErr bool
}

// ChainConfig describes an honest chain. Every produced block carries one
// aggregated vote from the first Participants(slot) validators for the parent
// block, whenever such a vote is valid.
type ChainConfig struct {
	Name         string
	Validators   int
	Slots        uint64
	Skip         func(slot uint64) bool
	Participants func(slot uint64) int
}

// Genesis returns a genesis state with deterministic validator keys.
func Genesis(validators int) (*types.State, error) {
	if validators <= 0 || validators > int(types.ValidatorRegistryLimit) {
		return nil, fmt.Errorf("validator count %d out of range", validators)
	}
	vals := make([]*types.Validator, validators)
	for i := range vals {
		var seed [8]byte
		binary.LittleEndian.PutUint64(seed[:], uint64(i))
		att := sha256.Sum256(append([]byte("attestation"), seed[:]...))
		prop := sha256.Sum256(append([]byte("proposal"), seed[:]...))
		vals[i] = &types.Validator{AttestationPubkey: att, ProposalPubkey: prop, Index: uint64(i)}
	}
	bodyRoot, err := (&types.BlockBody{}).HashTreeRoot()
	if err != nil {
		return nil, err
	}
	return &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1_000},
		LatestBlockHeader:        &types.BlockHeader{BodyRoot: bodyRoot},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		Validators:               vals,
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}, nil
}

// Chain builds the chain described by cfg and returns one accept case per
// block, each with the pre-state the block applies to.
func Chain(cfg ChainConfig) ([]Case, error) {
	state, err := Genesis(cfg.Validators)
	if err != nil {
		return nil, err
	}
	var cases []Case
	for slot := uint64(1); slot <= cfg.Slots; slot++ {
		if cfg.Skip != nil && cfg.Skip(slot) {
			continue
		}
		participants := cfg.Validators
		if cfg.Participants != nil {
			participants = cfg.Participants(slot)
		}
		block, post, err := NextBlock(state, slot, participants)
		if err != nil {
			return nil, fmt.Errorf("%s slot %d: %w", cfg.Name, slot, err)
		}
		cases = append(cases, Case{
			Name:  fmt.Sprintf("%s/slot-%03d", cfg.Name, slot),
			Pre:   state,
			Block: block,
		})
		state = post
	}
	return cases, nil
}

// NextBlock builds a valid block at slot on top of state, voting with the
// first participants validators when a vote for the parent is valid. It
// returns the block and the resulting post-state; state is not modified.
func NextBlock(state *types.State, slot uint64, participants int) (*types.Block, *types.State, error) {
	n := state.NumValidators()
	view, err := state.Clone()
	if err != nil {
		return nil, nil, err
	}
	if err := statetransition.ProcessSlots(view, slot); err != nil {
		return nil, nil, err
	}
	parentRoot, err := view.LatestBlockHeader.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	block := &types.Block{
		Slot:          slot,
		ProposerIndex: types.ProposerIndex(slot, n),
		ParentRoot:    parentRoot,
		Body:          &types.BlockBody{},
	}
	// Process the header alone to see the history and justified checkpoint the
	// block's votes will be judged against.
	if err := statetransition.ProcessBlockHeader(view, block); err != nil {
		return nil, nil, err
	}
	parent := &types.Checkpoint{Root: parentRoot, Slot: state.LatestBlockHeader.Slot}
	if participants > 0 &&
		statetransition.IsValidVote(view, view.LatestJustified, parent) &&
		statetransition.HeadMatchesChain(view, parent) {
		ids := make([]uint64, min(participants, int(n)))
		for i := range ids {
			ids[i] = uint64(i)
		}
		block.Body.Attestations = []*types.AggregatedAttestation{{
			AggregationBits: types.BitlistFromIndices(ids),
			Data: &types.AttestationData{
				Slot:   parent.Slot,
				Head:   parent,
				Target: parent,
				Source: view.LatestJustified,
			},
		}}
	}

	post, err := state.Clone()
	if err != nil {
		return nil, nil, err
	}
	if err := statetransition.ProcessSlots(post, slot); err != nil {
		return nil, nil, err
	}
	if err := statetransition.ProcessBlock(post, block); err != nil {
		return nil, nil, err
	}
	if block.StateRoot, err = post.HashTreeRoot(); err != nil {
		return nil, nil, err
	}
	return block, post, nil
}
