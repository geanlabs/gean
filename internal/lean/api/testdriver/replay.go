package testdriver

import (
	"bytes"
	"fmt"

	"github.com/geanlabs/gean/internal/core/storage"
	"github.com/geanlabs/gean/internal/lean/attestation"
	"github.com/geanlabs/gean/internal/lean/forkchoice"
	"github.com/geanlabs/gean/internal/lean/importer"
	"github.com/geanlabs/gean/internal/lean/specfixtures"
	"github.com/geanlabs/gean/internal/lean/store"
	"github.com/geanlabs/gean/internal/lean/types"
)

// Replay drives a fork-choice fixture's steps through gean's store, fork
// choice, block import and attestation validation. The hive driver serves it
// over HTTP and the leanSpec fixture tests call it directly, so both replay
// fixtures the same way.
type Replay struct {
	Store *store.ConsensusStore
	FC    *forkchoice.ForkChoice
	// LabelRoots maps the fixture's block labels to the roots of the blocks
	// that carried them. leanSpec labels the anchor "genesis" at any slot.
	LabelRoots map[string][32]byte
}

// NewReplay starts a replay from an anchor block and its post-state. The block
// must commit to the state, as a node checks before trusting an anchor.
func NewReplay(anchorState *types.State, anchorBlock *types.Block) (*Replay, error) {
	anchorRoot, err := anchorBlock.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("anchorBlock.hashTreeRoot: %w", err)
	}
	stateRoot, err := anchorState.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("anchorState.hashTreeRoot: %w", err)
	}
	if stateRoot != anchorBlock.StateRoot {
		return nil, fmt.Errorf("anchor state-root mismatch: block=0x%x state=0x%x", anchorBlock.StateRoot, stateRoot)
	}
	anchorHeader, err := anchorBlock.Header()
	if err != nil {
		return nil, fmt.Errorf("anchorBlock.header: %w", err)
	}
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	if err := s.PutAnchor(anchorRoot, anchorHeader, anchorState); err != nil {
		return nil, err
	}
	if err := s.StorePendingBlock(anchorRoot, &types.SignedBlock{Block: anchorBlock}); err != nil {
		return nil, fmt.Errorf("store anchor block: %w", err)
	}
	return &Replay{
		Store:      s,
		FC:         forkchoice.New(anchorBlock.Slot, anchorRoot, anchorBlock.ParentRoot),
		LabelRoots: map[string][32]byte{"genesis": anchorRoot},
	}, nil
}

// Step applies one fixture step. A non-nil error means gean did not accept it.
func (r *Replay) Step(step *specfixtures.ForkChoiceStep) error {
	var err error
	switch step.StepType {
	case "tick":
		err = r.tick(step)
	case "block":
		err = r.block(step)
	case "attestation":
		err = r.attestation(step)
	case "gossipAggregatedAttestation":
		err = r.aggregatedAttestation(step)
	case "checks":
	default:
		err = fmt.Errorf("unknown stepType %q", step.StepType)
	}
	return err
}

// mockedProofSentinel prefixes the placeholder aggregation proofs the upstream
// fixtures carry for weight-only vectors. A proof bearing it is not real
// crypto, so its verification is skipped. Production verification never sees
// this: a forged proof could otherwise carry the prefix.
var mockedProofSentinel = []byte("\x00MOCKED-AGGREGATION-PROOF\x00")

func carriesMockedProof(proof []byte) bool {
	return bytes.HasPrefix(proof, mockedProofSentinel)
}

// earliestAdmissibleInterval is the lowest store time at which a slot-N vote
// clears the future-admission check: admission allows
// slot*INTERVALS_PER_SLOT <= time + GOSSIP_DISPARITY_INTERVALS, so the vote is
// admissible one disparity interval before its slot starts. Advancing exactly to
// this interval, never to the slot start, keeps a following tick's promotion
// from being swallowed.
func earliestAdmissibleInterval(slot uint64) uint64 {
	start := slot * types.IntervalsPerSlot
	if start < types.GossipDisparityIntervals {
		return 0
	}
	return start - types.GossipDisparityIntervals
}

// tick advances the clock to the step's time or interval.
func (r *Replay) tick(step *specfixtures.ForkChoiceStep) error {
	genesisTime := r.Store.Config().GenesisTime
	var target uint64
	switch {
	case step.Time != nil:
		target = types.TotalIntervals(genesisTime, *step.Time*1000)
	case step.Interval != nil:
		target = *step.Interval
	default:
		return fmt.Errorf("tick step missing time and interval")
	}

	r.advance(target, step.HasProposal != nil && *step.HasProposal)
	// A tick to at or behind the clock still pins the time, so a later time
	// check reads the fixture's value.
	if target < r.Store.Time() {
		r.Store.SetTime(target)
	}
	return nil
}

// advance runs each interval up to target as leanSpec's on_tick does, a
// proposal signalled only on the last: store.OnTick promotes new votes, the head
// moves at intervals 0 and 4, and the safe target at interval 3. It follows
// leanSpec's timeline; the engine also prunes at interval 3.
func (r *Replay) advance(target uint64, hasProposal bool) {
	genesisTime := r.Store.Config().GenesisTime
	for r.Store.Time() < target {
		next := r.Store.Time() + 1
		store.OnTick(r.Store, genesisTime*1000+next*types.MillisecondsPerInterval, hasProposal && next == target)
		switch next % types.IntervalsPerSlot {
		case 0, 4:
			r.updateHead()
		case 3:
			store.UpdateSafeTarget(r.Store, r.FC)
		}
	}
}

func (r *Replay) block(step *specfixtures.ForkChoiceStep) error {
	if step.Block == nil {
		return fmt.Errorf("block step missing block payload")
	}
	block, err := step.Block.ToBlock()
	if err != nil {
		return err
	}

	signedBlock := &types.SignedBlock{Block: block, Proof: &types.MultiMessageAggregate{}}

	// Advance the clock to this block's slot unless the fixture delivers it ahead
	// of the clock (tickToSlot=false). The clock gates attestation future-validation,
	// so it must reach the slot before subsequent attestations validate.
	// leanSpec ticks to the slot with the block as its proposal.
	if step.TickToSlot == nil || *step.TickToSlot {
		r.advance(block.Slot*types.IntervalsPerSlot, true)
	}

	if err := importer.OnBlockWithoutVerification(r.Store, signedBlock); err != nil {
		return err
	}

	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		return err
	}
	if step.Block.BlockRootLabel != "" {
		r.LabelRoots[step.Block.BlockRootLabel] = blockRoot
	}
	r.FC.OnBlock(block.Slot, blockRoot, block.ParentRoot)

	// Seed the block's on-chain aggregated attestations into the known pool with
	// their participant sets so the votes carry fork-choice weight. Block import
	// alone stores the data weightless, which would mis-resolve weight-driven
	// reorgs. Only participants are read during head computation, so a non-empty
	// placeholder proof suffices.
	if block.Body != nil {
		for _, att := range block.Body.Attestations {
			if att == nil || att.Data == nil || types.BitlistCount(att.AggregationBits) == 0 {
				continue
			}
			dataRoot, err := att.Data.HashTreeRoot()
			if err != nil {
				continue
			}
			r.Store.KnownPayloads.Push(dataRoot, att.Data, &types.SingleMessageAggregate{
				Participants: att.AggregationBits,
				Proof:        []byte{0x01},
			})
		}
	}

	r.updateHead()

	// Promote new payloads to known, then reflect the just-promoted votes into the
	// known tracker — the next head update re-derives known votes from the promoted
	// pool, so a vote gossiped as "new" would otherwise stay absent from the tracker.
	r.Store.PromoteNewToKnown()
	for vid, data := range r.Store.ExtractLatestKnownAttestations() {
		r.FC.SetKnownVote(vid, data.Head.Root, data.Slot, data)
	}
	return nil
}

func (r *Replay) attestation(step *specfixtures.ForkChoiceStep) error {
	if step.Attestation == nil {
		return fmt.Errorf("attestation step missing attestation payload")
	}
	attData, err := step.Attestation.Data.ToAttestationData()
	if err != nil {
		return err
	}

	// For valid steps advance just enough to clear the future-admission check,
	// stopping one disparity interval before the slot start. Invalid steps keep the
	// current time so the rejection path still fires.
	if step.Valid {
		if minTime := earliestAdmissibleInterval(attData.Slot); r.Store.Time() < minTime {
			r.Store.SetTime(minTime)
		}
	}

	dataRoot, err := attData.HashTreeRoot()
	if err != nil {
		return err
	}
	signature, err := specfixtures.ParseHexBytes(step.Attestation.Signature)
	if err != nil {
		return fmt.Errorf("decode signature hex: %w", err)
	}

	// Data/bounds validation always runs so rejection fixtures exercise the
	// validator. The crypto check is skipped only for mocked placeholder proofs.
	// A non-nil error reports the step as not accepted; the simulator compares
	// that against the fixture's expected outcome.
	if err := attestation.ValidateAttestationData(r.Store, attData); err != nil {
		return err
	}
	if !carriesMockedProof(signature) {
		if err := attestation.VerifyGossipAttestation(r.Store, step.Attestation.ValidatorID, attData, dataRoot, signature); err != nil {
			return err
		}
	}

	participants := types.BitlistFromIndices([]uint64{step.Attestation.ValidatorID})
	r.Store.NewPayloads.Push(dataRoot, attData, &types.SingleMessageAggregate{Participants: participants})

	var sig [types.SignatureSize]byte
	copy(sig[:], signature)
	r.Store.AttestationSignatures.Insert(dataRoot, attData, step.Attestation.ValidatorID, sig)

	// Gossip lands in the new pool only; the head keeps reflecting the known pool
	// until a slot-boundary tick promotes these votes, so recompute from the known
	// pool here without promoting.
	r.updateHead()
	return nil
}

func (r *Replay) aggregatedAttestation(step *specfixtures.ForkChoiceStep) error {
	if step.Attestation == nil {
		return fmt.Errorf("gossipAggregatedAttestation step missing attestation payload")
	}
	attData, err := step.Attestation.Data.ToAttestationData()
	if err != nil {
		return err
	}

	if step.Valid {
		if minTime := earliestAdmissibleInterval(attData.Slot); r.Store.Time() < minTime {
			r.Store.SetTime(minTime)
		}
	}

	var participants []byte
	var proofData []byte
	if step.Attestation.Proof != nil {
		if participants, err = specfixtures.ParseBoolBitlist(step.Attestation.Proof.Participants.Data); err != nil {
			return fmt.Errorf("proof.participants: %w", err)
		}
		if proofData, err = specfixtures.ParseHexBytes(step.Attestation.Proof.Proof.Data); err != nil {
			return fmt.Errorf("proof.proofData: %w", err)
		}
	}
	dataRoot, err := attData.HashTreeRoot()
	if err != nil {
		return err
	}

	if err := attestation.ValidateAttestationData(r.Store, attData); err != nil {
		return err
	}
	if !carriesMockedProof(proofData) {
		if err := attestation.VerifyAggregatedGossipAttestation(r.Store, attData, participants, proofData); err != nil {
			return err
		}
	}

	r.Store.NewPayloads.Push(dataRoot, attData, &types.SingleMessageAggregate{Participants: participants, Proof: proofData})

	r.updateHead()
	return nil
}

// updateHead recomputes the head from the known votes and keeps the finalized
// checkpoint on the head's chain, as the engine does: a higher-finalized fork
// that loses head selection must not latch finalization.
func (r *Replay) updateHead() {
	head := store.ComputeHead(r.Store, r.FC)
	r.Store.SetHead(head)
	if derived := store.DeriveFinalizedFromHead(r.Store, head); derived != nil {
		r.Store.SetLatestFinalized(derived)
	}
}
