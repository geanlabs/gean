package store

import (
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/xmss"
)

const (
	aggregatedPayloadCap = 0
	newPayloadCap        = 0
)

type ConsensusStore struct {
	Backend               storage.Backend
	NewPayloads           *PayloadBuffer
	KnownPayloads         *PayloadBuffer
	AttestationSignatures AttestationSignatureMap
	PubKeyCache           *xmss.PubKeyCache
}

func NewConsensusStore(backend storage.Backend) *ConsensusStore {
	return &ConsensusStore{
		Backend:               backend,
		NewPayloads:           NewPayloadBuffer(newPayloadCap),
		KnownPayloads:         NewPayloadBuffer(aggregatedPayloadCap),
		AttestationSignatures: NewAttestationSignatureMap(),
		PubKeyCache:           xmss.NewPubKeyCache(),
	}
}
