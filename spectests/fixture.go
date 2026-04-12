//go:build spectests

package spectests

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/geanlabs/gean/types"
)

// TestFixture wraps the top-level JSON (key = test name, value = test data).
type TestFixture map[string]StateTransitionTest

type StateTransitionTest struct {
	Network                string         `json:"network"`
	LeanEnv                string         `json:"leanEnv"`
	Pre                    TestState      `json:"pre"`
	Blocks                 []TestBlock    `json:"blocks"`
	Post                   *TestPostState `json:"post"`
	ExpectException        string         `json:"expectException"`
	ExpectExceptionMessage string         `json:"expectExceptionMessage"`
}

type TestState struct {
	Config                   TestConfig        `json:"config"`
	Slot                     uint64            `json:"slot"`
	LatestBlockHeader        TestBlockHeader   `json:"latestBlockHeader"`
	LatestJustified          TestCheckpoint    `json:"latestJustified"`
	LatestFinalized          TestCheckpoint    `json:"latestFinalized"`
	HistoricalBlockHashes    TestDataList      `json:"historicalBlockHashes"`
	JustifiedSlots           TestDataList      `json:"justifiedSlots"`
	Validators               TestValidatorList `json:"validators"`
	JustificationsRoots      TestDataList      `json:"justificationsRoots"`
	JustificationsValidators TestDataList      `json:"justificationsValidators"`
}

type TestConfig struct {
	GenesisTime uint64 `json:"genesisTime"`
}

type TestBlockHeader struct {
	Slot          uint64 `json:"slot"`
	ProposerIndex uint64 `json:"proposerIndex"`
	ParentRoot    string `json:"parentRoot"`
	StateRoot     string `json:"stateRoot"`
	BodyRoot      string `json:"bodyRoot"`
}

type TestCheckpoint struct {
	Root string `json:"root"`
	Slot uint64 `json:"slot"`
}

type TestBlock struct {
	Slot          uint64        `json:"slot"`
	ProposerIndex uint64        `json:"proposerIndex"`
	ParentRoot    string        `json:"parentRoot"`
	StateRoot     string        `json:"stateRoot"`
	Body          TestBlockBody `json:"body"`
}

type TestBlockBody struct {
	Attestations TestDataList `json:"attestations"`
}

// TestDataList wraps the { "data": [...] } pattern used in fixtures.
type TestDataList struct {
	Data []json.RawMessage `json:"data"`
}

type TestValidator struct {
	// Devnet-4 dual-key fields (camelCase per spec CamelModel).
	AttestationPubkey string `json:"attestationPubkey"`
	ProposalPubkey    string `json:"proposalPubkey"`
	// Legacy devnet-3 single-key field (fallback).
	Pubkey string `json:"pubkey"`
	Index  uint64 `json:"index"`
}

type TestValidatorList struct {
	Data []TestValidator `json:"data"`
}

type TestAggregatedAttestation struct {
	AggregationBits TestDataList `json:"aggregationBits"`
	Data            TestAttData  `json:"data"`
}

type TestAttData struct {
	Slot   uint64         `json:"slot"`
	Head   TestCheckpoint `json:"head"`
	Target TestCheckpoint `json:"target"`
	Source TestCheckpoint `json:"source"`
}

type TestPostState struct {
	Slot                           *uint64       `json:"slot"`
	LatestBlockHeaderSlot          *uint64       `json:"latestBlockHeaderSlot"`
	LatestBlockHeaderStateRoot     *string       `json:"latestBlockHeaderStateRoot"`
	LatestBlockHeaderProposerIndex *uint64       `json:"latestBlockHeaderProposerIndex"`
	LatestBlockHeaderParentRoot    *string       `json:"latestBlockHeaderParentRoot"`
	LatestBlockHeaderBodyRoot      *string       `json:"latestBlockHeaderBodyRoot"`
	LatestJustifiedSlot            *uint64       `json:"latestJustifiedSlot"`
	LatestJustifiedRoot            *string       `json:"latestJustifiedRoot"`
	LatestFinalizedSlot            *uint64       `json:"latestFinalizedSlot"`
	LatestFinalizedRoot            *string       `json:"latestFinalizedRoot"`
	HistoricalBlockHashesCount     *uint64       `json:"historicalBlockHashesCount"`
	ValidatorCount                 *uint64       `json:"validatorCount"`
	ConfigGenesisTime              *uint64       `json:"configGenesisTime"`
	HistoricalBlockHashes          *TestDataList `json:"historicalBlockHashes"`
	JustifiedSlots                 *TestDataList `json:"justifiedSlots"`
	JustificationsRoots            *TestDataList `json:"justificationsRoots"`
	JustificationsValidators       *TestDataList `json:"justificationsValidators"`
}

func parseHexRoot(s string) [32]byte {
	s = strings.TrimPrefix(s, "0x")
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(fmt.Sprintf("parseHexRoot: invalid hex %q: %v", s, err))
	}
	var root [32]byte
	copy(root[:], b)
	return root
}

func parseHexBytes(s string) []byte {
	s = strings.TrimPrefix(s, "0x")
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(fmt.Sprintf("parseHexBytes: invalid hex %q: %v", s, err))
	}
	return b
}

func parseHexPubkey(s string) [types.PubkeySize]byte {
	b := parseHexBytes(s)
	var pk [types.PubkeySize]byte
	copy(pk[:], b)
	return pk
}

// ToState converts test JSON pre-state to gean's types.State.
func (ts *TestState) ToState() *types.State {
	state := &types.State{
		Config: &types.ChainConfig{
			GenesisTime: ts.Config.GenesisTime,
		},
		Slot: ts.Slot,
		LatestBlockHeader: &types.BlockHeader{
			Slot:          ts.LatestBlockHeader.Slot,
			ProposerIndex: ts.LatestBlockHeader.ProposerIndex,
			ParentRoot:    parseHexRoot(ts.LatestBlockHeader.ParentRoot),
			StateRoot:     parseHexRoot(ts.LatestBlockHeader.StateRoot),
			BodyRoot:      parseHexRoot(ts.LatestBlockHeader.BodyRoot),
		},
		LatestJustified: &types.Checkpoint{
			Root: parseHexRoot(ts.LatestJustified.Root),
			Slot: ts.LatestJustified.Slot,
		},
		LatestFinalized: &types.Checkpoint{
			Root: parseHexRoot(ts.LatestFinalized.Root),
			Slot: ts.LatestFinalized.Slot,
		},
	}

	// Validators — supports both devnet-4 dual-key and legacy single-key fixtures.
	for _, v := range ts.Validators.Data {
		var attPk, propPk [types.PubkeySize]byte
		if v.AttestationPubkey != "" {
			attPk = parseHexPubkey(v.AttestationPubkey)
			propPk = parseHexPubkey(v.ProposalPubkey)
		} else {
			// Legacy single-key fallback.
			pk := parseHexPubkey(v.Pubkey)
			attPk = pk
			propPk = pk
		}
		state.Validators = append(state.Validators, &types.Validator{
			AttestationPubkey: attPk,
			ProposalPubkey:    propPk,
			Index:             v.Index,
		})
	}

	// HistoricalBlockHashes: array of hex strings
	for _, raw := range ts.HistoricalBlockHashes.Data {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			panic(fmt.Sprintf("HistoricalBlockHashes: %v", err))
		}
		b := parseHexBytes(s)
		// Ensure 32 bytes
		h := make([]byte, 32)
		copy(h, b)
		state.HistoricalBlockHashes = append(state.HistoricalBlockHashes, h)
	}

	// JustifiedSlots: array of boolean values representing bit positions.
	// Convert to SSZ bitlist.
	state.JustifiedSlots = parseBoolBitlist(ts.JustifiedSlots.Data)

	// JustificationsRoots: array of hex strings
	for _, raw := range ts.JustificationsRoots.Data {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			panic(fmt.Sprintf("JustificationsRoots: %v", err))
		}
		b := parseHexBytes(s)
		h := make([]byte, 32)
		copy(h, b)
		state.JustificationsRoots = append(state.JustificationsRoots, h)
	}

	// JustificationsValidators: array of booleans -> bitlist
	state.JustificationsValidators = parseBoolBitlist(ts.JustificationsValidators.Data)

	return state
}

// parseBoolBitlist converts a JSON array of booleans to an SSZ bitlist.
// An empty array returns a minimal bitlist (just the delimiter bit).
func parseBoolBitlist(data []json.RawMessage) []byte {
	length := uint64(len(data))
	if length == 0 {
		return types.NewBitlistSSZ(0)
	}
	bl := types.NewBitlistSSZ(length)
	for i, raw := range data {
		var val bool
		if err := json.Unmarshal(raw, &val); err != nil {
			// Try parsing as integer (0/1)
			var intVal int
			if err2 := json.Unmarshal(raw, &intVal); err2 != nil {
				panic(fmt.Sprintf("parseBoolBitlist index %d: %v / %v", i, err, err2))
			}
			val = intVal != 0
		}
		if val {
			types.BitlistSet(bl, uint64(i))
		}
	}
	return bl
}

// ToBlock converts test JSON block to gean's types.Block.
func (tb *TestBlock) ToBlock() *types.Block {
	block := &types.Block{
		Slot:          tb.Slot,
		ProposerIndex: tb.ProposerIndex,
		ParentRoot:    parseHexRoot(tb.ParentRoot),
		StateRoot:     parseHexRoot(tb.StateRoot),
		Body: &types.BlockBody{
			Attestations: make([]*types.AggregatedAttestation, 0),
		},
	}

	for _, raw := range tb.Body.Attestations.Data {
		var ta TestAggregatedAttestation
		if err := json.Unmarshal(raw, &ta); err != nil {
			panic(fmt.Sprintf("attestation unmarshal: %v", err))
		}

		att := &types.AggregatedAttestation{
			AggregationBits: parseBoolBitlist(ta.AggregationBits.Data),
			Data: &types.AttestationData{
				Slot: ta.Data.Slot,
				Head: &types.Checkpoint{
					Root: parseHexRoot(ta.Data.Head.Root),
					Slot: ta.Data.Head.Slot,
				},
				Target: &types.Checkpoint{
					Root: parseHexRoot(ta.Data.Target.Root),
					Slot: ta.Data.Target.Slot,
				},
				Source: &types.Checkpoint{
					Root: parseHexRoot(ta.Data.Source.Root),
					Slot: ta.Data.Source.Slot,
				},
			},
		}
		block.Body.Attestations = append(block.Body.Attestations, att)
	}

	return block
}

// Validate checks fields in the post-state expectation against actual state.
// Only non-nil fields are checked (selective validation).
func (tp *TestPostState) Validate(state *types.State) error {
	if tp.Slot != nil {
		if state.Slot != *tp.Slot {
			return fmt.Errorf("slot: got %d, want %d", state.Slot, *tp.Slot)
		}
	}

	if tp.LatestBlockHeaderSlot != nil {
		if state.LatestBlockHeader.Slot != *tp.LatestBlockHeaderSlot {
			return fmt.Errorf("latestBlockHeader.slot: got %d, want %d",
				state.LatestBlockHeader.Slot, *tp.LatestBlockHeaderSlot)
		}
	}

	if tp.LatestBlockHeaderStateRoot != nil {
		want := parseHexRoot(*tp.LatestBlockHeaderStateRoot)
		if state.LatestBlockHeader.StateRoot != want {
			return fmt.Errorf("latestBlockHeader.stateRoot: got 0x%x, want 0x%x",
				state.LatestBlockHeader.StateRoot, want)
		}
	}

	if tp.LatestBlockHeaderProposerIndex != nil {
		if state.LatestBlockHeader.ProposerIndex != *tp.LatestBlockHeaderProposerIndex {
			return fmt.Errorf("latestBlockHeader.proposerIndex: got %d, want %d",
				state.LatestBlockHeader.ProposerIndex, *tp.LatestBlockHeaderProposerIndex)
		}
	}

	if tp.LatestBlockHeaderParentRoot != nil {
		want := parseHexRoot(*tp.LatestBlockHeaderParentRoot)
		if state.LatestBlockHeader.ParentRoot != want {
			return fmt.Errorf("latestBlockHeader.parentRoot: got 0x%x, want 0x%x",
				state.LatestBlockHeader.ParentRoot, want)
		}
	}

	if tp.LatestBlockHeaderBodyRoot != nil {
		want := parseHexRoot(*tp.LatestBlockHeaderBodyRoot)
		if state.LatestBlockHeader.BodyRoot != want {
			return fmt.Errorf("latestBlockHeader.bodyRoot: got 0x%x, want 0x%x",
				state.LatestBlockHeader.BodyRoot, want)
		}
	}

	if tp.LatestJustifiedSlot != nil {
		if state.LatestJustified.Slot != *tp.LatestJustifiedSlot {
			return fmt.Errorf("latestJustified.slot: got %d, want %d",
				state.LatestJustified.Slot, *tp.LatestJustifiedSlot)
		}
	}

	if tp.LatestJustifiedRoot != nil {
		want := parseHexRoot(*tp.LatestJustifiedRoot)
		if state.LatestJustified.Root != want {
			return fmt.Errorf("latestJustified.root: got 0x%x, want 0x%x",
				state.LatestJustified.Root, want)
		}
	}

	if tp.LatestFinalizedSlot != nil {
		if state.LatestFinalized.Slot != *tp.LatestFinalizedSlot {
			return fmt.Errorf("latestFinalized.slot: got %d, want %d",
				state.LatestFinalized.Slot, *tp.LatestFinalizedSlot)
		}
	}

	if tp.LatestFinalizedRoot != nil {
		want := parseHexRoot(*tp.LatestFinalizedRoot)
		if state.LatestFinalized.Root != want {
			return fmt.Errorf("latestFinalized.root: got 0x%x, want 0x%x",
				state.LatestFinalized.Root, want)
		}
	}

	if tp.HistoricalBlockHashesCount != nil {
		got := uint64(len(state.HistoricalBlockHashes))
		if got != *tp.HistoricalBlockHashesCount {
			return fmt.Errorf("historicalBlockHashes count: got %d, want %d",
				got, *tp.HistoricalBlockHashesCount)
		}
	}

	if tp.ValidatorCount != nil {
		got := state.NumValidators()
		if got != *tp.ValidatorCount {
			return fmt.Errorf("validator count: got %d, want %d", got, *tp.ValidatorCount)
		}
	}

	if tp.ConfigGenesisTime != nil {
		if state.Config.GenesisTime != *tp.ConfigGenesisTime {
			return fmt.Errorf("config.genesisTime: got %d, want %d",
				state.Config.GenesisTime, *tp.ConfigGenesisTime)
		}
	}

	if tp.HistoricalBlockHashes != nil {
		wantLen := len(tp.HistoricalBlockHashes.Data)
		gotLen := len(state.HistoricalBlockHashes)
		if gotLen != wantLen {
			return fmt.Errorf("historicalBlockHashes length: got %d, want %d", gotLen, wantLen)
		}
		for i, raw := range tp.HistoricalBlockHashes.Data {
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				return fmt.Errorf("historicalBlockHashes[%d] unmarshal: %v", i, err)
			}
			want := parseHexBytes(s)
			got := state.HistoricalBlockHashes[i]
			if !bytesEqual(got, want) {
				return fmt.Errorf("historicalBlockHashes[%d]: got 0x%x, want 0x%x", i, got, want)
			}
		}
	}

	if tp.JustifiedSlots != nil {
		wantBitlist := parseBoolBitlist(tp.JustifiedSlots.Data)
		if !bytesEqual(state.JustifiedSlots, wantBitlist) {
			return fmt.Errorf("justifiedSlots mismatch: got %x, want %x",
				state.JustifiedSlots, wantBitlist)
		}
	}

	if tp.JustificationsRoots != nil {
		wantLen := len(tp.JustificationsRoots.Data)
		gotLen := len(state.JustificationsRoots)
		if gotLen != wantLen {
			return fmt.Errorf("justificationsRoots length: got %d, want %d", gotLen, wantLen)
		}
		for i, raw := range tp.JustificationsRoots.Data {
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				return fmt.Errorf("justificationsRoots[%d] unmarshal: %v", i, err)
			}
			want := parseHexBytes(s)
			got := state.JustificationsRoots[i]
			if !bytesEqual(got, want) {
				return fmt.Errorf("justificationsRoots[%d]: got 0x%x, want 0x%x", i, got, want)
			}
		}
	}

	if tp.JustificationsValidators != nil {
		wantBitlist := parseBoolBitlist(tp.JustificationsValidators.Data)
		if !bytesEqual(state.JustificationsValidators, wantBitlist) {
			return fmt.Errorf("justificationsValidators mismatch: got %x, want %x",
				state.JustificationsValidators, wantBitlist)
		}
	}

	return nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
