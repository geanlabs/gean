package blockprocessor

import (
	"fmt"
	"runtime"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
	"golang.org/x/sync/errgroup"
)

type verifyJob struct {
	attIdx    int
	proofData []byte
	pubkeys   []xmss.CPubKey
	dataRoot  [32]byte
	slot      uint32
}

func buildVerifyJobs(s *store.ConsensusStore, block *types.Block, sigs *types.BlockSignatures) ([]verifyJob, error) {
	if s == nil {
		return nil, fmt.Errorf("consensus store is nil")
	}
	if block == nil || block.Body == nil {
		return nil, fmt.Errorf("malformed block")
	}
	if sigs == nil {
		return nil, &store.StoreError{
			Kind:    store.ErrAttestationSignatureMismatch,
			Message: "block signatures missing",
		}
	}
	if len(sigs.AttestationSignatures) != len(block.Body.Attestations) {
		return nil, &store.StoreError{
			Kind: store.ErrAttestationSignatureMismatch,
			Message: fmt.Sprintf(
				"block has %d attestations but %d attestation signatures",
				len(block.Body.Attestations),
				len(sigs.AttestationSignatures),
			),
		}
	}

	jobs := make([]verifyJob, 0, len(block.Body.Attestations))
	for i, att := range block.Body.Attestations {
		if !validAttestationShape(att) {
			return nil, fmt.Errorf("malformed block attestation")
		}

		proof := sigs.AttestationSignatures[i]
		if err := validateProofForAttestation(i, att, proof); err != nil {
			return nil, err
		}

		dataRoot, err := att.Data.HashTreeRoot()
		if err != nil {
			return nil, fmt.Errorf("hash attestation data: %w", err)
		}
		slot, err := slot32(att.Data.Slot)
		if err != nil {
			return nil, &store.StoreError{Kind: store.ErrSignatureDecodingFailed, Message: fmt.Sprintf("attestation %d slot: %v", i, err)}
		}

		targetState := s.GetState(att.Data.Target.Root)
		if targetState == nil {
			return nil, &store.StoreError{
				Kind:    store.ErrMissingTargetState,
				Message: fmt.Sprintf("attestation %d target state missing: 0x%x", i, att.Data.Target.Root),
			}
		}

		pubkeys, err := participantPubkeys(s, targetState, proof)
		if err != nil {
			return nil, err
		}

		jobs = append(jobs, verifyJob{
			attIdx:    i,
			proofData: proof.ProofData,
			pubkeys:   pubkeys,
			dataRoot:  dataRoot,
			slot:      slot,
		})
	}
	return jobs, nil
}

func runVerifyJobs(jobs []verifyJob) error {
	var g errgroup.Group
	g.SetLimit(runtime.GOMAXPROCS(0))
	for _, job := range jobs {
		job := job
		g.Go(func() error {
			if err := xmss.VerifyAggregatedSignature(job.proofData, job.pubkeys, job.dataRoot, job.slot); err != nil {
				return &store.StoreError{Kind: store.ErrAggregateVerificationFailed, Message: fmt.Sprintf("attestation %d proof: %v", job.attIdx, err)}
			}
			return nil
		})
	}
	return g.Wait()
}
