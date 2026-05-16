package specfixtures

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/geanlabs/gean/types"
)

// ParseHexRoot decodes a "0x"-prefixed (or bare) 32-byte hex string. Returns
// an error rather than panicking so HTTP handlers can surface a clean 400.
func ParseHexRoot(s string) ([32]byte, error) {
	var root [32]byte
	b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		return root, fmt.Errorf("invalid hex root %q: %w", s, err)
	}
	copy(root[:], b)
	return root, nil
}

// ParseHexBytes decodes a "0x"-prefixed (or bare) hex string into a byte slice
// of whatever length the input encodes.
func ParseHexBytes(s string) ([]byte, error) {
	b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		return nil, fmt.Errorf("invalid hex %q: %w", s, err)
	}
	return b, nil
}

// ParseHexPubkey decodes a 52-byte XMSS public key.
func ParseHexPubkey(s string) ([types.PubkeySize]byte, error) {
	var pk [types.PubkeySize]byte
	b, err := ParseHexBytes(s)
	if err != nil {
		return pk, err
	}
	if len(b) > types.PubkeySize {
		return pk, fmt.Errorf("pubkey too long: got %d bytes, want %d", len(b), types.PubkeySize)
	}
	copy(pk[:], b)
	return pk, nil
}

// ParseHexSignature decodes a 2536-byte XMSS signature.
func ParseHexSignature(s string) ([types.SignatureSize]byte, error) {
	var sig [types.SignatureSize]byte
	b, err := ParseHexBytes(s)
	if err != nil {
		return sig, err
	}
	if len(b) > types.SignatureSize {
		return sig, fmt.Errorf("signature too long: got %d bytes, want %d", len(b), types.SignatureSize)
	}
	copy(sig[:], b)
	return sig, nil
}

// ParseBoolBitlist converts a JSON array of bool-or-int values into an SSZ
// bitlist. Accepts both `true/false` and `1/0` per fixture convention.
func ParseBoolBitlist(data []json.RawMessage) ([]byte, error) {
	length := uint64(len(data))
	if length == 0 {
		return types.NewBitlistSSZ(0), nil
	}
	bl := types.NewBitlistSSZ(length)
	for i, raw := range data {
		var val bool
		if err := json.Unmarshal(raw, &val); err != nil {
			var intVal int
			if err2 := json.Unmarshal(raw, &intVal); err2 != nil {
				return nil, fmt.Errorf("bitlist index %d: not bool or int: %w / %w", i, err, err2)
			}
			val = intVal != 0
		}
		if val {
			types.BitlistSet(bl, uint64(i))
		}
	}
	return bl, nil
}

// ToState converts a fixture state to gean's runtime types.State. The
// embedded LatestJustified/LatestFinalized are kept verbatim from the
// fixture; the caller is responsible for any anchor-checkpoint substitution
// (see fork-choice runners that re-pin both to the anchor root).
func (ts *TestState) ToState() (*types.State, error) {
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
		Config:            &types.ChainConfig{GenesisTime: ts.Config.GenesisTime},
		Slot:              ts.Slot,
		LatestBlockHeader: &types.BlockHeader{Slot: ts.LatestBlockHeader.Slot, ProposerIndex: ts.LatestBlockHeader.ProposerIndex, ParentRoot: parentRoot, StateRoot: stateRoot, BodyRoot: bodyRoot},
		LatestJustified:   &types.Checkpoint{Root: justifiedRoot, Slot: ts.LatestJustified.Slot},
		LatestFinalized:   &types.Checkpoint{Root: finalizedRoot, Slot: ts.LatestFinalized.Slot},
	}

	for i, v := range ts.Validators.Data {
		var attPk, propPk [types.PubkeySize]byte
		if v.AttestationPubkey != "" {
			if attPk, err = ParseHexPubkey(v.AttestationPubkey); err != nil {
				return nil, fmt.Errorf("validators[%d].attestationPubkey: %w", i, err)
			}
			if propPk, err = ParseHexPubkey(v.ProposalPubkey); err != nil {
				return nil, fmt.Errorf("validators[%d].proposalPubkey: %w", i, err)
			}
		} else {
			pk, perr := ParseHexPubkey(v.Pubkey)
			if perr != nil {
				return nil, fmt.Errorf("validators[%d].pubkey: %w", i, perr)
			}
			attPk = pk
			propPk = pk
		}
		state.Validators = append(state.Validators, &types.Validator{AttestationPubkey: attPk, ProposalPubkey: propPk, Index: v.Index})
	}

	for i, raw := range ts.HistoricalBlockHashes.Data {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("historicalBlockHashes[%d]: %w", i, err)
		}
		b, perr := ParseHexBytes(s)
		if perr != nil {
			return nil, fmt.Errorf("historicalBlockHashes[%d]: %w", i, perr)
		}
		h := make([]byte, 32)
		copy(h, b)
		state.HistoricalBlockHashes = append(state.HistoricalBlockHashes, h)
	}

	if state.JustifiedSlots, err = ParseBoolBitlist(ts.JustifiedSlots.Data); err != nil {
		return nil, fmt.Errorf("justifiedSlots: %w", err)
	}

	for i, raw := range ts.JustificationsRoots.Data {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("justificationsRoots[%d]: %w", i, err)
		}
		b, perr := ParseHexBytes(s)
		if perr != nil {
			return nil, fmt.Errorf("justificationsRoots[%d]: %w", i, perr)
		}
		h := make([]byte, 32)
		copy(h, b)
		state.JustificationsRoots = append(state.JustificationsRoots, h)
	}

	if state.JustificationsValidators, err = ParseBoolBitlist(ts.JustificationsValidators.Data); err != nil {
		return nil, fmt.Errorf("justificationsValidators: %w", err)
	}

	return state, nil
}

// ToBlock converts a fixture block to types.Block, including all aggregated
// attestations in its body.
func (tb *TestBlock) ToBlock() (*types.Block, error) {
	parentRoot, err := ParseHexRoot(tb.ParentRoot)
	if err != nil {
		return nil, fmt.Errorf("block.parentRoot: %w", err)
	}
	stateRoot, err := ParseHexRoot(tb.StateRoot)
	if err != nil {
		return nil, fmt.Errorf("block.stateRoot: %w", err)
	}

	block := &types.Block{
		Slot:          tb.Slot,
		ProposerIndex: tb.ProposerIndex,
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body:          &types.BlockBody{Attestations: make([]*types.AggregatedAttestation, 0)},
	}

	for i, raw := range tb.Body.Attestations.Data {
		var ta TestAggregatedAttestation
		if err := json.Unmarshal(raw, &ta); err != nil {
			return nil, fmt.Errorf("block.body.attestations[%d]: %w", i, err)
		}
		att, err := ta.ToAggregatedAttestation()
		if err != nil {
			return nil, fmt.Errorf("block.body.attestations[%d]: %w", i, err)
		}
		block.Body.Attestations = append(block.Body.Attestations, att)
	}

	return block, nil
}

// ToAggregatedAttestation converts a fixture aggregated attestation to its
// runtime form.
func (ta *TestAggregatedAttestation) ToAggregatedAttestation() (*types.AggregatedAttestation, error) {
	bits, err := ParseBoolBitlist(ta.AggregationBits.Data)
	if err != nil {
		return nil, fmt.Errorf("aggregationBits: %w", err)
	}
	data, err := ta.Data.ToAttestationData()
	if err != nil {
		return nil, err
	}
	return &types.AggregatedAttestation{AggregationBits: bits, Data: data}, nil
}

// ToAttestationData converts a fixture attestation data record.
func (tad *TestAttData) ToAttestationData() (*types.AttestationData, error) {
	headRoot, err := ParseHexRoot(tad.Head.Root)
	if err != nil {
		return nil, fmt.Errorf("data.head.root: %w", err)
	}
	targetRoot, err := ParseHexRoot(tad.Target.Root)
	if err != nil {
		return nil, fmt.Errorf("data.target.root: %w", err)
	}
	sourceRoot, err := ParseHexRoot(tad.Source.Root)
	if err != nil {
		return nil, fmt.Errorf("data.source.root: %w", err)
	}
	return &types.AttestationData{
		Slot:   tad.Slot,
		Head:   &types.Checkpoint{Root: headRoot, Slot: tad.Head.Slot},
		Target: &types.Checkpoint{Root: targetRoot, Slot: tad.Target.Slot},
		Source: &types.Checkpoint{Root: sourceRoot, Slot: tad.Source.Slot},
	}, nil
}

// ToSignedBlock converts a fixture signed block envelope to its runtime form.
// Handles both the devnet-4 flat layout and the legacy nested-message layout.
func (sb *FixtureSignedBlock) ToSignedBlock() (*types.SignedBlock, error) {
	srcBlock := sb.Block
	if (srcBlock.Slot == 0 && srcBlock.ParentRoot == "") && sb.Message != nil {
		srcBlock = sb.Message.Block
	}
	block, err := srcBlock.ToBlock()
	if err != nil {
		return nil, fmt.Errorf("signedBlock.block: %w", err)
	}

	proposerSig, err := ParseHexSignature(sb.Signature.ProposerSignature)
	if err != nil {
		return nil, fmt.Errorf("signedBlock.signature.proposerSignature: %w", err)
	}

	var attSigs []*types.AggregatedSignatureProof
	for i, proof := range sb.Signature.AttestationSignatures.Data {
		participants, perr := ParseBoolBitlist(proof.Participants.Data)
		if perr != nil {
			return nil, fmt.Errorf("signedBlock.signature.attestationSignatures[%d].participants: %w", i, perr)
		}
		proofData, perr := ParseHexBytes(proof.ProofData.Data)
		if perr != nil {
			return nil, fmt.Errorf("signedBlock.signature.attestationSignatures[%d].proofData: %w", i, perr)
		}
		attSigs = append(attSigs, &types.AggregatedSignatureProof{Participants: participants, ProofData: proofData})
	}

	return &types.SignedBlock{
		Block: block,
		Signature: &types.BlockSignatures{
			ProposerSignature:     proposerSig,
			AttestationSignatures: attSigs,
		},
	}, nil
}
