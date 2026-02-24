package spectests

import (
	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/types"
)

// convertState converts a fixture JSON state to a domain State.
func convertState(fs FixtureState) *types.State {
	config := &types.Config{GenesisTime: fs.Config.GenesisTime}

	header := &types.BlockHeader{
		Slot:          fs.LatestBlockHeader.Slot,
		ProposerIndex: fs.LatestBlockHeader.ProposerIndex,
		ParentRoot:    [32]byte(fs.LatestBlockHeader.ParentRoot),
		StateRoot:     [32]byte(fs.LatestBlockHeader.StateRoot),
		BodyRoot:      [32]byte(fs.LatestBlockHeader.BodyRoot),
	}

	latestJustified := &types.Checkpoint{
		Root: [32]byte(fs.LatestJustified.Root),
		Slot: fs.LatestJustified.Slot,
	}
	latestFinalized := &types.Checkpoint{
		Root: [32]byte(fs.LatestFinalized.Root),
		Slot: fs.LatestFinalized.Slot,
	}

	hashes := make([][32]byte, len(fs.HistoricalBlockHashes.Data))
	for i, h := range fs.HistoricalBlockHashes.Data {
		hashes[i] = [32]byte(h)
	}

	justifiedSlots := buildBitlist(fs.JustifiedSlots.Data)

	validators := make([]*types.Validator, len(fs.Validators.Data))
	for i, v := range fs.Validators.Data {
		validators[i] = &types.Validator{
			Pubkey: [52]byte(v.Pubkey),
			Index:  v.Index,
		}
	}

	justificationsRoots := make([][32]byte, len(fs.JustificationsRoots.Data))
	for i, r := range fs.JustificationsRoots.Data {
		justificationsRoots[i] = [32]byte(r)
	}

	justificationsValidators := buildBoolBitlist(fs.JustificationsValidators.Data)

	return &types.State{
		Config:                   config,
		Slot:                     fs.Slot,
		LatestBlockHeader:        header,
		LatestJustified:          latestJustified,
		LatestFinalized:          latestFinalized,
		HistoricalBlockHashes:    hashes,
		JustifiedSlots:           justifiedSlots,
		Validators:               validators,
		JustificationsRoots:      justificationsRoots,
		JustificationsValidators: justificationsValidators,
	}
}

// convertBlock converts a fixture JSON block to a domain Block.
func convertBlock(fb FixtureBlock) *types.Block {
	atts := make([]*types.Attestation, len(fb.Body.Attestations.Data))
	for i, a := range fb.Body.Attestations.Data {
		atts[i] = convertAttestation(a)
	}
	return &types.Block{
		Slot:          fb.Slot,
		ProposerIndex: fb.ProposerIndex,
		ParentRoot:    [32]byte(fb.ParentRoot),
		StateRoot:     [32]byte(fb.StateRoot),
		Body:          &types.BlockBody{Attestations: atts},
	}
}

// convertAttestation converts a fixture attestation to a domain Attestation.
func convertAttestation(fa FixtureAttestation) *types.Attestation {
	return &types.Attestation{
		ValidatorID: fa.ValidatorID,
		Data: &types.AttestationData{
			Slot: fa.Data.Slot,
			Head: &types.Checkpoint{
				Root: [32]byte(fa.Data.Head.Root),
				Slot: fa.Data.Head.Slot,
			},
			Target: &types.Checkpoint{
				Root: [32]byte(fa.Data.Target.Root),
				Slot: fa.Data.Target.Slot,
			},
			Source: &types.Checkpoint{
				Root: [32]byte(fa.Data.Source.Root),
				Slot: fa.Data.Source.Slot,
			},
		},
	}
}

// convertSignedAttestation converts a fixture signed attestation to a domain SignedAttestation.
// Uses a zero signature since fixture tests skip signature verification.
func convertSignedAttestation(fa FixtureSignedAttestation) *types.SignedAttestation {
	return &types.SignedAttestation{
		Message: &types.Attestation{
			ValidatorID: fa.ValidatorID,
			Data: &types.AttestationData{
				Slot: fa.Data.Slot,
				Head: &types.Checkpoint{
					Root: [32]byte(fa.Data.Head.Root),
					Slot: fa.Data.Head.Slot,
				},
				Target: &types.Checkpoint{
					Root: [32]byte(fa.Data.Target.Root),
					Slot: fa.Data.Target.Slot,
				},
				Source: &types.Checkpoint{
					Root: [32]byte(fa.Data.Source.Root),
					Slot: fa.Data.Source.Slot,
				},
			},
		},
	}
}

// buildBitlist converts a slice of uint64 (0 or 1 values) to an SSZ bitlist.
func buildBitlist(bits []uint64) []byte {
	bl := []byte{0x01} // empty bitlist with sentinel
	for _, b := range bits {
		bl = statetransition.AppendBit(bl, b != 0)
	}
	return bl
}

// buildBoolBitlist converts a slice of bools to an SSZ bitlist.
func buildBoolBitlist(bits []bool) []byte {
	bl := []byte{0x01} // empty bitlist with sentinel
	for _, b := range bits {
		bl = statetransition.AppendBit(bl, b)
	}
	return bl
}

// makeZeroSignatures creates a slice of zero-valued 3112-byte XMSS signatures.
func makeZeroSignatures(count int) [][3112]byte {
	sigs := make([][3112]byte, count)
	return sigs
}
