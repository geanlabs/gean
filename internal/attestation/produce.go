package attestation

import (
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func ProduceAttestationData(s *store.ConsensusStore, slot uint64) *types.AttestationData {
	headRoot := s.Head()
	headState := s.GetState(headRoot)
	if headState == nil || headState.LatestBlockHeader == nil {
		return nil
	}

	source := s.LatestJustified()
	if headState.LatestBlockHeader.Slot == 0 {
		source = &types.Checkpoint{
			Root: headRoot,
			Slot: source.Slot,
		}
	}

	headHeader := s.GetBlockHeader(headRoot)
	if headHeader == nil {
		return nil
	}
	headCheckpoint := &types.Checkpoint{
		Root: headRoot,
		Slot: headHeader.Slot,
	}

	target := GetAttestationTarget(s)

	logger.Info(logger.Chain, "ProduceAttestation: slot=%d head=0x%x source=0x%x/%d target=0x%x/%d",
		slot, headRoot, source.Root, source.Slot, target.Root, target.Slot)

	return &types.AttestationData{
		Slot:   slot,
		Head:   headCheckpoint,
		Target: target,
		Source: source,
	}
}
