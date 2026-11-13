package blockprocessor

import (
	"github.com/geanlabs/gean/lean/consensus/types"
	"github.com/geanlabs/gean/lean/internal/pools"
)

func importBlockAttestations(known *pools.PayloadBuffer, signedBlock *types.SignedBlock) {
	if known == nil || signedBlock == nil ||
		signedBlock.Block == nil || signedBlock.Block.Body == nil {
		return
	}
	for _, att := range signedBlock.Block.Body.Attestations {
		if !validAttestationShape(att) {
			continue
		}
		root, err := att.Data.HashTreeRoot()
		if err == nil {
			known.PushData(root, att.Data)
		}
	}
}
