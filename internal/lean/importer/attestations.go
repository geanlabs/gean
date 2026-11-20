package importer

import (
	"github.com/geanlabs/gean/internal/lean/store"
	"github.com/geanlabs/gean/internal/lean/types"
)

func importBlockAttestations(s *store.ConsensusStore, signedBlock *types.SignedBlock) {
	if s == nil || s.KnownPayloads == nil || signedBlock == nil ||
		signedBlock.Block == nil || signedBlock.Block.Body == nil {
		return
	}
	for _, att := range signedBlock.Block.Body.Attestations {
		if !validAttestationShape(att) {
			continue
		}
		root, err := att.Data.HashTreeRoot()
		if err == nil {
			s.KnownPayloads.PushData(root, att.Data)
		}
	}
}
