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

	"github.com/geanlabs/gean/logger"
	"github.com/geanlabs/gean/node"
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

// Fixture types for signature verification tests.

type sigFixture map[string]sigTest

type sigTest struct {
	Network     string   `json:"network"`
	LeanEnv     string   `json:"leanEnv"`
	AnchorState sigState `json:"anchorState"`
	SignedBlock sigSBA   `json:"signedBlock"`
	// Legacy devnet-3 field (fallback).
	SignedBlockWithAttestation sigSBA  `json:"signedBlockWithAttestation"`
	ExpectException            *string `json:"expectException"`
}

type sigState struct {
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

type sigSBA struct {
	// Devnet-4 flat structure: block + signature directly.
	Block     fcBlock      `json:"block"`
	Signature sigSignature `json:"signature"`
	// Legacy devnet-3 wrapper (fallback).
	Message *sigMessage `json:"message,omitempty"`
}

type sigMessage struct {
	Block fcBlock `json:"block"`
}

type sigSignature struct {
	ProposerSignature     string        `json:"proposerSignature"`
	AttestationSignatures sigAttSigList `json:"attestationSignatures"`
}

type sigAttSigList struct {
	Data []sigAttSigProof `json:"data"`
}

type sigAttSigProof struct {
	Participants sigBoolList  `json:"participants"`
	ProofData    sigProofData `json:"proofData"`
}

type sigBoolList struct {
	Data []json.RawMessage `json:"data"`
}

type sigProofData struct {
	Data string `json:"data"`
}

// Parsing helpers (duplicated from fork choice tests to keep file self-contained).

func sigParseHexRoot(s string) [32]byte {
	s = strings.TrimPrefix(s, "0x")
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(fmt.Sprintf("sigParseHexRoot: invalid hex %q: %v", s, err))
	}
	var root [32]byte
	copy(root[:], b)
	return root
}

func sigParseHexBytes(s string) []byte {
	s = strings.TrimPrefix(s, "0x")
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(fmt.Sprintf("sigParseHexBytes: invalid hex %q: %v", s, err))
	}
	return b
}

func sigParseHexPubkey(s string) [types.PubkeySize]byte {
	b := sigParseHexBytes(s)
	var pk [types.PubkeySize]byte
	copy(pk[:], b)
	return pk
}

func sigParseBoolBitlist(data []json.RawMessage) []byte {
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
				panic(fmt.Sprintf("sigParseBoolBitlist index %d: %v / %v", i, err, err2))
			}
			val = intVal != 0
		}
		if val {
			types.BitlistSet(bl, uint64(i))
		}
	}
	return bl
}

func sigParseHexSignature(s string) [types.SignatureSize]byte {
	b := sigParseHexBytes(s)
	var sig [types.SignatureSize]byte
	copy(sig[:], b)
	return sig
}

// toState converts fixture anchor state to types.State.
func (fs *sigState) toState() *types.State {
	state := &types.State{
		Config: &types.ChainConfig{
			GenesisTime: fs.Config.GenesisTime,
		},
		Slot: fs.Slot,
		LatestBlockHeader: &types.BlockHeader{
			Slot:          fs.LatestBlockHeader.Slot,
			ProposerIndex: fs.LatestBlockHeader.ProposerIndex,
			ParentRoot:    sigParseHexRoot(fs.LatestBlockHeader.ParentRoot),
			StateRoot:     sigParseHexRoot(fs.LatestBlockHeader.StateRoot),
			BodyRoot:      sigParseHexRoot(fs.LatestBlockHeader.BodyRoot),
		},
		LatestJustified: &types.Checkpoint{
			Root: sigParseHexRoot(fs.LatestJustified.Root),
			Slot: fs.LatestJustified.Slot,
		},
		LatestFinalized: &types.Checkpoint{
			Root: sigParseHexRoot(fs.LatestFinalized.Root),
			Slot: fs.LatestFinalized.Slot,
		},
	}

	for _, v := range fs.Validators.Data {
		var attPk, propPk [types.PubkeySize]byte
		if v.AttestationPubkey != "" {
			attPk = sigParseHexPubkey(v.AttestationPubkey)
			propPk = sigParseHexPubkey(v.ProposalPubkey)
		} else {
			pk := sigParseHexPubkey(v.Pubkey)
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
		b := sigParseHexBytes(s)
		h := make([]byte, 32)
		copy(h, b)
		state.HistoricalBlockHashes = append(state.HistoricalBlockHashes, h)
	}

	state.JustifiedSlots = sigParseBoolBitlist(fs.JustifiedSlots.Data)

	for _, raw := range fs.JustificationsRoots.Data {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			panic(fmt.Sprintf("JustificationsRoots: %v", err))
		}
		b := sigParseHexBytes(s)
		h := make([]byte, 32)
		copy(h, b)
		state.JustificationsRoots = append(state.JustificationsRoots, h)
	}

	state.JustificationsValidators = sigParseBoolBitlist(fs.JustificationsValidators.Data)

	return state
}

// toSignedBlock converts fixture signed block to types.SignedBlock.
func (sba *sigSBA) toSignedBlock() *types.SignedBlock {
	// Devnet-4: block is directly on sba. Legacy: block is in sba.Message.
	var srcBlock fcBlock
	if sba.Block.Slot > 0 || sba.Block.ParentRoot != "" {
		srcBlock = sba.Block
	} else if sba.Message != nil {
		srcBlock = sba.Message.Block
	}
	block := srcBlock.toBlock()

	proposerSig := sigParseHexSignature(sba.Signature.ProposerSignature)

	var attSigs []*types.AggregatedSignatureProof
	for _, proof := range sba.Signature.AttestationSignatures.Data {
		participants := sigParseBoolBitlist(proof.Participants.Data)
		proofData := sigParseHexBytes(proof.ProofData.Data)
		attSigs = append(attSigs, &types.AggregatedSignatureProof{
			Participants: participants,
			ProofData:    proofData,
		})
	}

	return &types.SignedBlock{
		Block: block,
		Signature: &types.BlockSignatures{
			ProposerSignature:     proposerSig,
			AttestationSignatures: attSigs,
		},
	}
}

// Test runner.

func TestSpecSignatures(t *testing.T) {
	logger.Quiet = true
	defer func() { logger.Quiet = false }()

	fixtureDir := "../leanSpec/fixtures/consensus/verify_signatures"

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

			var fixture sigFixture
			if err := json.Unmarshal(data, &fixture); err != nil {
				t.Fatalf("unmarshalling %s: %v", file, err)
			}

			for testName, tt := range fixture {
				tt := tt
				t.Run(testName, func(t *testing.T) {
					runSignatureTest(t, &tt)
				})
			}
		})
	}
}

func runSignatureTest(t *testing.T, tt *sigTest) {
	t.Helper()

	// 1. Convert anchor state.
	anchorState := tt.AnchorState.toState()

	// 2. Fill state root in header if zero (genesis case), then compute anchor block root.
	// Matches initStoreFromState in main.go.
	stateRoot, _ := anchorState.HashTreeRoot()
	header := anchorState.LatestBlockHeader
	if header.StateRoot == types.ZeroRoot {
		header.StateRoot = stateRoot
	}
	anchorRoot, err := header.HashTreeRoot()
	if err != nil {
		t.Fatalf("computing anchor block root: %v", err)
	}

	// 3. Initialize store with in-memory backend.
	backend := storage.NewInMemoryBackend()
	s := node.NewConsensusStore(backend)

	s.SetConfig(anchorState.Config)
	s.InsertBlockHeader(anchorRoot, header)
	s.InsertState(anchorRoot, anchorState)
	s.InsertLiveChainEntry(header.Slot, anchorRoot, header.ParentRoot)
	s.SetHead(anchorRoot)
	s.SetLatestJustified(&types.Checkpoint{Root: anchorRoot, Slot: header.Slot})
	s.SetLatestFinalized(&types.Checkpoint{Root: anchorRoot, Slot: header.Slot})

	// 4. Convert fixture signed block.
	sba := tt.SignedBlock
	if sba.Block.Slot == 0 && sba.Block.ParentRoot == "" {
		sba = tt.SignedBlockWithAttestation // legacy fallback
	}
	signedBlock := sba.toSignedBlock()

	// 5. Call OnBlock WITH signature verification.
	err = node.OnBlock(s, signedBlock, nil)

	// 6. Check result against expectation.
	expectFailure := tt.ExpectException != nil

	if expectFailure {
		if err == nil {
			t.Fatalf("expected failure (expectException=%q) but OnBlock succeeded", *tt.ExpectException)
		}
	} else {
		if err != nil {
			t.Fatalf("expected success but OnBlock failed: %v", err)
		}
	}
}
