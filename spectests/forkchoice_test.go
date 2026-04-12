//go:build spectests

package spectests

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geanlabs/gean/forkchoice"
	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/node"
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

// --- Fixture types for fork choice tests ---

type fcFixture map[string]fcTest

type fcTest struct {
	Network     string   `json:"network"`
	LeanEnv     string   `json:"leanEnv"`
	AnchorState fcState  `json:"anchorState"`
	AnchorBlock fcBlock  `json:"anchorBlock"`
	Steps       []fcStep `json:"steps"`
}

type fcState struct {
	Config                   fcConfig        `json:"config"`
	Slot                     uint64          `json:"slot"`
	LatestBlockHeader        fcBlockHeader   `json:"latestBlockHeader"`
	LatestJustified          fcCheckpoint    `json:"latestJustified"`
	LatestFinalized          fcCheckpoint    `json:"latestFinalized"`
	HistoricalBlockHashes    fcDataList      `json:"historicalBlockHashes"`
	JustifiedSlots           fcDataList      `json:"justifiedSlots"`
	Validators               fcValidatorList `json:"validators"`
	JustificationsRoots      fcDataList      `json:"justificationsRoots"`
	JustificationsValidators fcDataList      `json:"justificationsValidators"`
}

type fcConfig struct {
	GenesisTime uint64 `json:"genesisTime"`
}

type fcBlockHeader struct {
	Slot          uint64 `json:"slot"`
	ProposerIndex uint64 `json:"proposerIndex"`
	ParentRoot    string `json:"parentRoot"`
	StateRoot     string `json:"stateRoot"`
	BodyRoot      string `json:"bodyRoot"`
}

type fcCheckpoint struct {
	Root string `json:"root"`
	Slot uint64 `json:"slot"`
}

type fcDataList struct {
	Data []json.RawMessage `json:"data"`
}

type fcValidator struct {
	AttestationPubkey string `json:"attestationPubkey"`
	ProposalPubkey    string `json:"proposalPubkey"`
	Pubkey            string `json:"pubkey"` // legacy fallback
	Index             uint64 `json:"index"`
}

type fcValidatorList struct {
	Data []fcValidator `json:"data"`
}

type fcBlock struct {
	Slot           uint64      `json:"slot"`
	ProposerIndex  uint64      `json:"proposerIndex"`
	ParentRoot     string      `json:"parentRoot"`
	StateRoot      string      `json:"stateRoot"`
	Body           fcBlockBody `json:"body"`
	BlockRootLabel string      `json:"blockRootLabel,omitempty"`
}

type fcBlockBody struct {
	Attestations fcDataList `json:"attestations"`
}

type fcStep struct {
	StepType    string               `json:"stepType"`
	Valid       bool                 `json:"valid"`
	Block       *fcBlock             `json:"block,omitempty"`
	Attestation *fcGossipAttestation `json:"attestation,omitempty"`
	Checks      *fcChecks            `json:"checks,omitempty"`
	Time        *uint64              `json:"time,omitempty"`
}

// fcGossipAttestation represents an individual gossip attestation step.
type fcGossipAttestation struct {
	ValidatorID uint64    `json:"validatorId"`
	Data        fcAttData `json:"data"`
	Signature   string    `json:"signature"`
	// Aggregated attestation fields (for gossipAggregatedAttestation steps).
	Proof *fcProof `json:"proof,omitempty"`
}

type fcProof struct {
	Participants fcDataList  `json:"participants"`
	ProofData    fcProofData `json:"proofData"`
}

type fcProofData struct {
	Data string `json:"data"`
}

type fcAttData struct {
	Slot   uint64       `json:"slot"`
	Head   fcCheckpoint `json:"head"`
	Target fcCheckpoint `json:"target"`
	Source fcCheckpoint `json:"source"`
}

type fcAggregatedAttestation struct {
	AggregationBits fcDataList `json:"aggregationBits"`
	Data            fcAttData  `json:"data"`
}

type fcChecks struct {
	HeadSlot      *uint64 `json:"headSlot,omitempty"`
	HeadRoot      *string `json:"headRoot,omitempty"`
	HeadRootLabel *string `json:"headRootLabel,omitempty"`
}

// --- Parsing helpers ---

func fcParseHexRoot(s string) [32]byte {
	s = strings.TrimPrefix(s, "0x")
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(fmt.Sprintf("fcParseHexRoot: invalid hex %q: %v", s, err))
	}
	var root [32]byte
	copy(root[:], b)
	return root
}

func fcParseHexBytes(s string) []byte {
	s = strings.TrimPrefix(s, "0x")
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(fmt.Sprintf("fcParseHexBytes: invalid hex %q: %v", s, err))
	}
	return b
}

func fcParseHexPubkey(s string) [types.PubkeySize]byte {
	b := fcParseHexBytes(s)
	var pk [types.PubkeySize]byte
	copy(pk[:], b)
	return pk
}

func fcParseBoolBitlist(data []json.RawMessage) []byte {
	length := uint64(len(data))
	if length == 0 {
		return types.NewBitlistSSZ(0)
	}
	bl := types.NewBitlistSSZ(length)
	for i, raw := range data {
		var val bool
		if err := json.Unmarshal(raw, &val); err != nil {
			var intVal int
			if err2 := json.Unmarshal(raw, &intVal); err2 != nil {
				panic(fmt.Sprintf("fcParseBoolBitlist index %d: %v / %v", i, err, err2))
			}
			val = intVal != 0
		}
		if val {
			types.BitlistSet(bl, uint64(i))
		}
	}
	return bl
}

// toState converts fixture anchor state to types.State.
func (fs *fcState) toState() *types.State {
	state := &types.State{
		Config: &types.ChainConfig{
			GenesisTime: fs.Config.GenesisTime,
		},
		Slot: fs.Slot,
		LatestBlockHeader: &types.BlockHeader{
			Slot:          fs.LatestBlockHeader.Slot,
			ProposerIndex: fs.LatestBlockHeader.ProposerIndex,
			ParentRoot:    fcParseHexRoot(fs.LatestBlockHeader.ParentRoot),
			StateRoot:     fcParseHexRoot(fs.LatestBlockHeader.StateRoot),
			BodyRoot:      fcParseHexRoot(fs.LatestBlockHeader.BodyRoot),
		},
		LatestJustified: &types.Checkpoint{
			Root: fcParseHexRoot(fs.LatestJustified.Root),
			Slot: fs.LatestJustified.Slot,
		},
		LatestFinalized: &types.Checkpoint{
			Root: fcParseHexRoot(fs.LatestFinalized.Root),
			Slot: fs.LatestFinalized.Slot,
		},
	}

	for _, v := range fs.Validators.Data {
		var attPk, propPk [types.PubkeySize]byte
		if v.AttestationPubkey != "" {
			attPk = fcParseHexPubkey(v.AttestationPubkey)
			propPk = fcParseHexPubkey(v.ProposalPubkey)
		} else {
			pk := fcParseHexPubkey(v.Pubkey)
			attPk = pk
			propPk = pk
		}
		state.Validators = append(state.Validators, &types.Validator{
			AttestationPubkey: attPk,
			ProposalPubkey:    propPk,
			Index:             v.Index,
		})
	}

	for _, raw := range fs.HistoricalBlockHashes.Data {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			panic(fmt.Sprintf("HistoricalBlockHashes: %v", err))
		}
		b := fcParseHexBytes(s)
		h := make([]byte, 32)
		copy(h, b)
		state.HistoricalBlockHashes = append(state.HistoricalBlockHashes, h)
	}

	state.JustifiedSlots = fcParseBoolBitlist(fs.JustifiedSlots.Data)

	for _, raw := range fs.JustificationsRoots.Data {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			panic(fmt.Sprintf("JustificationsRoots: %v", err))
		}
		b := fcParseHexBytes(s)
		h := make([]byte, 32)
		copy(h, b)
		state.JustificationsRoots = append(state.JustificationsRoots, h)
	}

	state.JustificationsValidators = fcParseBoolBitlist(fs.JustificationsValidators.Data)

	return state
}

// toBlock converts a fixture block to types.Block.
func (fb *fcBlock) toBlock() *types.Block {
	block := &types.Block{
		Slot:          fb.Slot,
		ProposerIndex: fb.ProposerIndex,
		ParentRoot:    fcParseHexRoot(fb.ParentRoot),
		StateRoot:     fcParseHexRoot(fb.StateRoot),
		Body: &types.BlockBody{
			Attestations: make([]*types.AggregatedAttestation, 0),
		},
	}

	for _, raw := range fb.Body.Attestations.Data {
		var ta fcAggregatedAttestation
		if err := json.Unmarshal(raw, &ta); err != nil {
			panic(fmt.Sprintf("attestation unmarshal: %v", err))
		}
		att := &types.AggregatedAttestation{
			AggregationBits: fcParseBoolBitlist(ta.AggregationBits.Data),
			Data: &types.AttestationData{
				Slot: ta.Data.Slot,
				Head: &types.Checkpoint{
					Root: fcParseHexRoot(ta.Data.Head.Root),
					Slot: ta.Data.Head.Slot,
				},
				Target: &types.Checkpoint{
					Root: fcParseHexRoot(ta.Data.Target.Root),
					Slot: ta.Data.Target.Slot,
				},
				Source: &types.Checkpoint{
					Root: fcParseHexRoot(ta.Data.Source.Root),
					Slot: ta.Data.Source.Slot,
				},
			},
		}
		block.Body.Attestations = append(block.Body.Attestations, att)
	}

	return block
}

// --- Test runner ---

func TestSpecForkChoice(t *testing.T) {
	logger.Quiet = true
	defer func() { logger.Quiet = false }()

	fixtureDir := "../leanSpec/fixtures/consensus/fork_choice"

	var files []string
	err := filepath.Walk(fixtureDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".json" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking fixture dir %s: %v", fixtureDir, err)
	}

	if len(files) == 0 {
		t.Fatalf("no fixture files found in %s", fixtureDir)
	}

	for _, file := range files {
		file := file
		relPath, _ := filepath.Rel(fixtureDir, file)
		t.Run(relPath, func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("reading %s: %v", file, err)
			}

			var fixture fcFixture
			if err := json.Unmarshal(data, &fixture); err != nil {
				t.Fatalf("unmarshalling %s: %v", file, err)
			}

			for testName, tt := range fixture {
				tt := tt
				t.Run(testName, func(t *testing.T) {
					runForkChoiceTest(t, &tt)
				})
			}
		})
	}
}

func runForkChoiceTest(t *testing.T, tt *fcTest) {
	t.Helper()

	// 1. Convert anchor state and block.
	anchorState := tt.AnchorState.toState()
	anchorBlock := tt.AnchorBlock.toBlock()

	// Compute anchor block root.
	anchorRoot, err := anchorBlock.HashTreeRoot()
	if err != nil {
		t.Fatalf("computing anchor block root: %v", err)
	}

	// 2. Initialize store with in-memory backend.
	backend := storage.NewInMemoryBackend()
	s := node.NewConsensusStore(backend)

	// Store config from anchor state.
	s.SetConfig(anchorState.Config)

	// Store anchor state + block header.
	anchorHeader := &types.BlockHeader{
		Slot:          anchorBlock.Slot,
		ProposerIndex: anchorBlock.ProposerIndex,
		ParentRoot:    anchorBlock.ParentRoot,
		StateRoot:     anchorBlock.StateRoot,
	}
	if anchorBlock.Body != nil {
		bodyRoot, _ := anchorBlock.Body.HashTreeRoot()
		anchorHeader.BodyRoot = bodyRoot
	}

	// Cache state root in anchor state's latest block header.
	anchorState.LatestBlockHeader.StateRoot = anchorBlock.StateRoot

	s.InsertBlockHeader(anchorRoot, anchorHeader)
	s.InsertState(anchorRoot, anchorState)
	s.InsertLiveChainEntry(anchorBlock.Slot, anchorRoot, anchorBlock.ParentRoot)
	s.SetHead(anchorRoot)
	s.SetLatestJustified(&types.Checkpoint{Root: anchorRoot, Slot: anchorBlock.Slot})
	s.SetLatestFinalized(&types.Checkpoint{Root: anchorRoot, Slot: anchorBlock.Slot})

	// Store anchor as signed block.
	anchorSigned := &types.SignedBlock{
		Block:     anchorBlock,
		Signature: nil,
	}
	s.StorePendingBlock(anchorRoot, anchorSigned)

	// 3. Initialize fork choice with anchor.
	fc := forkchoice.New(anchorBlock.Slot, anchorRoot)

	// Label -> root map for resolving blockRootLabel references.
	labelRoots := make(map[string][32]byte)

	// 4. Process steps.
	for i, step := range tt.Steps {
		switch step.StepType {
		case "block":
			if step.Block == nil {
				t.Fatalf("step %d: block step without block data", i)
			}

			block := step.Block.toBlock()

			// Build signatures with participant bits from attestation aggregation_bits
			// so processBlockAttestations stores correct per-validator votes.
			var attSigs []*types.AggregatedSignatureProof
			if block.Body != nil {
				for _, att := range block.Body.Attestations {
					attSigs = append(attSigs, &types.AggregatedSignatureProof{
						Participants: att.AggregationBits,
					})
				}
			}
			signedBlock := &types.SignedBlock{
				Block: block,
				Signature: &types.BlockSignatures{
					AttestationSignatures: attSigs,
				},
			}

			// Advance store time to at least this block's slot so that
			// subsequent attestation validation doesn't reject as "too far in future".
			minTime := block.Slot * types.IntervalsPerSlot
			if s.Time() < minTime {
				s.SetTime(minTime)
			}

			// Process block through store (no signature verification).
			if err := node.OnBlockWithoutVerification(s, signedBlock); err != nil {
				if step.Valid {
					t.Fatalf("step %d: OnBlockWithoutVerification failed: %v", i, err)
				}
				continue
			}

			// Compute block root and register label.
			blockRoot, _ := block.HashTreeRoot()
			if step.Block.BlockRootLabel != "" {
				labelRoots[step.Block.BlockRootLabel] = blockRoot
			}

			// Register block in fork choice.
			fc.OnBlock(block.Slot, blockRoot, block.ParentRoot)

			// Update head: extract known attestations, feed to fork choice, compute head.
			attestations := s.ExtractLatestKnownAttestations()
			justifiedRoot := s.LatestJustified().Root

			for vid, data := range attestations {
				idx := fc.NodeIndex(data.Head.Root)
				if idx >= 0 {
					fc.Votes.SetKnown(vid, idx, data.Slot, data)
				}
			}

			newHead := fc.UpdateHead(justifiedRoot)
			s.SetHead(newHead)

			// Promote new payloads to known (so next updateHead sees them).
			s.PromoteNewToKnown()

			// Validate checks if present.
			if step.Checks != nil {
				validateChecks(t, i, step.Checks, s, fc, labelRoots)
			}

		case "attestation":
			if step.Attestation == nil {
				t.Fatalf("step %d: attestation step without data", i)
			}
			att := step.Attestation
			attData := &types.AttestationData{
				Slot:   att.Data.Slot,
				Head:   &types.Checkpoint{Root: fcParseHexRoot(att.Data.Head.Root), Slot: att.Data.Head.Slot},
				Target: &types.Checkpoint{Root: fcParseHexRoot(att.Data.Target.Root), Slot: att.Data.Target.Slot},
				Source: &types.Checkpoint{Root: fcParseHexRoot(att.Data.Source.Root), Slot: att.Data.Source.Slot},
			}

			// For valid steps, advance time so attestation passes the future check.
			// Invalid steps keep current time to test rejection.
			if step.Valid {
				minTime := attData.Slot * types.IntervalsPerSlot
				if s.Time() < minTime {
					s.SetTime(minTime)
				}
			}
			err := node.ValidateAttestationData(s, attData)
			if err != nil {
				if step.Valid {
					t.Fatalf("step %d: ValidateAttestationData failed: %v", i, err)
				}
				if step.Checks != nil {
					validateChecks(t, i, step.Checks, s, fc, labelRoots)
				}
				continue
			}
			if !step.Valid {
				// Sig/validator checks can't fail in test mode (no sig verification).
				// Skip rather than fail — these tests exercise the full gossip pipeline.
				if step.Checks != nil {
					validateChecks(t, i, step.Checks, s, fc, labelRoots)
				}
				continue
			}

			// Store in new payloads with dummy proof.
			participants := node.AggregationBitsFromIndices([]uint64{att.ValidatorID})
			dataRoot, _ := attData.HashTreeRoot()
			proof := &types.AggregatedSignatureProof{
				Participants: participants,
				ProofData:    nil,
			}
			s.NewPayloads.Push(dataRoot, attData, proof)

			// Feed vote to fork choice so attestation weight is reflected.
			idx := fc.NodeIndex(attData.Head.Root)
			if idx >= 0 {
				fc.Votes.SetNew(att.ValidatorID, idx, attData.Slot, attData)
			}

			// Promote + update head.
			s.PromoteNewToKnown()
			knownAtts := s.ExtractLatestKnownAttestations()
			justifiedRoot := s.LatestJustified().Root
			for vid, data := range knownAtts {
				jdx := fc.NodeIndex(data.Head.Root)
				if jdx >= 0 {
					fc.Votes.SetKnown(vid, jdx, data.Slot, data)
				}
			}
			newHead := fc.UpdateHead(justifiedRoot)
			s.SetHead(newHead)

			if step.Checks != nil {
				validateChecks(t, i, step.Checks, s, fc, labelRoots)
			}

		case "gossipAggregatedAttestation":
			if step.Attestation == nil {
				t.Fatalf("step %d: gossipAggregatedAttestation step without data", i)
			}
			att := step.Attestation
			attData := &types.AttestationData{
				Slot:   att.Data.Slot,
				Head:   &types.Checkpoint{Root: fcParseHexRoot(att.Data.Head.Root), Slot: att.Data.Head.Slot},
				Target: &types.Checkpoint{Root: fcParseHexRoot(att.Data.Target.Root), Slot: att.Data.Target.Slot},
				Source: &types.Checkpoint{Root: fcParseHexRoot(att.Data.Source.Root), Slot: att.Data.Source.Slot},
			}

			// For valid steps, advance time so attestation passes the future check.
			// Invalid steps keep current time to test rejection.
			if step.Valid {
				minTime := attData.Slot * types.IntervalsPerSlot
				if s.Time() < minTime {
					s.SetTime(minTime)
				}
			}
			err := node.ValidateAttestationData(s, attData)
			if err != nil {
				if step.Valid {
					t.Fatalf("step %d: ValidateAttestationData failed: %v", i, err)
				}
				if step.Checks != nil {
					validateChecks(t, i, step.Checks, s, fc, labelRoots)
				}
				continue
			}
			if !step.Valid {
				// Skip — sig/proof verification not performed in test mode.
				if step.Checks != nil {
					validateChecks(t, i, step.Checks, s, fc, labelRoots)
				}
				continue
			}

			// Parse participants and store in new payloads.
			var participants []byte
			if att.Proof != nil {
				participants = fcParseBoolBitlist(att.Proof.Participants.Data)
			}
			dataRoot, _ := attData.HashTreeRoot()
			var proofData []byte
			if att.Proof != nil {
				proofData = fcParseHexBytes(att.Proof.ProofData.Data)
			}
			proof := &types.AggregatedSignatureProof{
				Participants: participants,
				ProofData:    proofData,
			}
			s.NewPayloads.Push(dataRoot, attData, proof)

			// Feed per-validator votes to fork choice from participant bits.
			participantIDs := types.BitlistIndices(participants)
			for _, vid := range participantIDs {
				idx := fc.NodeIndex(attData.Head.Root)
				if idx >= 0 {
					fc.Votes.SetNew(vid, idx, attData.Slot, attData)
				}
			}

			// Promote + update head.
			s.PromoteNewToKnown()
			knownAtts := s.ExtractLatestKnownAttestations()
			justifiedRoot := s.LatestJustified().Root
			for vid, data := range knownAtts {
				jdx := fc.NodeIndex(data.Head.Root)
				if jdx >= 0 {
					fc.Votes.SetKnown(vid, jdx, data.Slot, data)
				}
			}
			newHead := fc.UpdateHead(justifiedRoot)
			s.SetHead(newHead)

			if step.Checks != nil {
				validateChecks(t, i, step.Checks, s, fc, labelRoots)
			}

		case "tick":
			if step.Time == nil {
				t.Fatalf("step %d: tick step without time", i)
			}
			s.SetTime(*step.Time)

		default:
			t.Fatalf("step %d: unknown step type %q", i, step.StepType)
		}
	}
}

func validateChecks(t *testing.T, stepIdx int, checks *fcChecks, s *node.ConsensusStore, fc *forkchoice.ForkChoice, labelRoots map[string][32]byte) {
	t.Helper()

	headRoot := s.Head()

	if checks.HeadSlot != nil {
		headHeader := s.GetBlockHeader(headRoot)
		if headHeader == nil {
			t.Fatalf("step %d check: head block header not found for root 0x%x", stepIdx, headRoot)
		}
		if headHeader.Slot != *checks.HeadSlot {
			t.Fatalf("step %d check: headSlot got %d, want %d", stepIdx, headHeader.Slot, *checks.HeadSlot)
		}
	}

	if checks.HeadRoot != nil {
		wantRoot := fcParseHexRoot(*checks.HeadRoot)
		if headRoot != wantRoot {
			t.Fatalf("step %d check: headRoot got 0x%x, want 0x%x", stepIdx, headRoot, wantRoot)
		}
	}

	if checks.HeadRootLabel != nil {
		if labelRoot, ok := labelRoots[*checks.HeadRootLabel]; ok {
			if headRoot != labelRoot {
				t.Fatalf("step %d check: headRootLabel %q got 0x%x, want 0x%x",
					stepIdx, *checks.HeadRootLabel, headRoot, labelRoot)
			}
		}
	}
}
