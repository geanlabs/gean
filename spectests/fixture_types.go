package spectests

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// HexRoot is a 32-byte root that deserializes from "0x..." hex strings.
type HexRoot [32]byte

func (h *HexRoot) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	s = strings.TrimPrefix(s, "0x")
	b, err := hex.DecodeString(s)
	if err != nil {
		return fmt.Errorf("invalid hex root: %w", err)
	}
	if len(b) != 32 {
		return fmt.Errorf("root must be 32 bytes, got %d", len(b))
	}
	copy(h[:], b)
	return nil
}

// HexPubkey is a 52-byte XMSS public key that deserializes from "0x..." hex strings.
type HexPubkey [52]byte

func (h *HexPubkey) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	s = strings.TrimPrefix(s, "0x")
	b, err := hex.DecodeString(s)
	if err != nil {
		return fmt.Errorf("invalid hex pubkey: %w", err)
	}
	if len(b) != 52 {
		return fmt.Errorf("pubkey must be 52 bytes, got %d", len(b))
	}
	copy(h[:], b)
	return nil
}

// Container wraps the {"data": [...]} pattern used in leanSpec JSON fixtures.
type Container[T any] struct {
	Data []T `json:"data"`
}

// --- Shared fixture types ---

type FixtureInfo struct {
	Hash          string `json:"hash"`
	Comment       string `json:"comment"`
	TestID        string `json:"testId"`
	Description   string `json:"description"`
	FixtureFormat string `json:"fixtureFormat"`
}

type FixtureConfig struct {
	GenesisTime uint64 `json:"genesisTime"`
}

type FixtureCheckpoint struct {
	Root HexRoot `json:"root"`
	Slot uint64  `json:"slot"`
}

type FixtureBlockHeader struct {
	Slot          uint64  `json:"slot"`
	ProposerIndex uint64  `json:"proposerIndex"`
	ParentRoot    HexRoot `json:"parentRoot"`
	StateRoot     HexRoot `json:"stateRoot"`
	BodyRoot      HexRoot `json:"bodyRoot"`
}

type FixtureValidator struct {
	Pubkey HexPubkey `json:"pubkey"`
	Index  uint64    `json:"index"`
}

type FixtureState struct {
	Config                   FixtureConfig               `json:"config"`
	Slot                     uint64                      `json:"slot"`
	LatestBlockHeader        FixtureBlockHeader          `json:"latestBlockHeader"`
	LatestJustified          FixtureCheckpoint           `json:"latestJustified"`
	LatestFinalized          FixtureCheckpoint           `json:"latestFinalized"`
	HistoricalBlockHashes    Container[HexRoot]          `json:"historicalBlockHashes"`
	JustifiedSlots           Container[uint64]           `json:"justifiedSlots"`
	Validators               Container[FixtureValidator] `json:"validators"`
	JustificationsRoots      Container[HexRoot]          `json:"justificationsRoots"`
	JustificationsValidators Container[bool]             `json:"justificationsValidators"`
}

type FixtureBlockBody struct {
	Attestations Container[FixtureAttestation] `json:"attestations"`
}

type FixtureBlock struct {
	Slot          uint64           `json:"slot"`
	ProposerIndex uint64           `json:"proposerIndex"`
	ParentRoot    HexRoot          `json:"parentRoot"`
	StateRoot     HexRoot          `json:"stateRoot"`
	Body          FixtureBlockBody `json:"body"`
}

type FixtureAttestationData struct {
	Slot   uint64            `json:"slot"`
	Head   FixtureCheckpoint `json:"head"`
	Target FixtureCheckpoint `json:"target"`
	Source FixtureCheckpoint `json:"source"`
}

type FixtureAttestation struct {
	ValidatorID uint64                 `json:"validatorId"`
	Data        FixtureAttestationData `json:"data"`
}

type FixtureSignedAttestation struct {
	ValidatorID uint64                 `json:"validatorId"`
	Data        FixtureAttestationData `json:"data"`
}

// --- State Transition fixture types ---

// StateTransitionFixture is the root JSON object: test_name -> test case.
type StateTransitionFixture map[string]StateTransitionTestCase

type StateTransitionTestCase struct {
	Network         string         `json:"network"`
	Pre             FixtureState   `json:"pre"`
	Blocks          []FixtureBlock `json:"blocks"`
	Post            *PostState     `json:"post"`
	ExpectException *string        `json:"expectException"`
	Info            FixtureInfo    `json:"_info"`
}

// PostState contains optional expected fields for selective validation.
// Nil pointer fields are not checked.
type PostState struct {
	Slot                           *uint64             `json:"slot"`
	LatestJustifiedSlot            *uint64             `json:"latestJustifiedSlot"`
	LatestJustifiedRoot            *HexRoot            `json:"latestJustifiedRoot"`
	LatestFinalizedSlot            *uint64             `json:"latestFinalizedSlot"`
	LatestFinalizedRoot            *HexRoot            `json:"latestFinalizedRoot"`
	ValidatorCount                 *uint64             `json:"validatorCount"`
	ConfigGenesisTime              *uint64             `json:"configGenesisTime"`
	LatestBlockHeaderSlot          *uint64             `json:"latestBlockHeaderSlot"`
	LatestBlockHeaderProposerIndex *uint64             `json:"latestBlockHeaderProposerIndex"`
	LatestBlockHeaderParentRoot    *HexRoot            `json:"latestBlockHeaderParentRoot"`
	LatestBlockHeaderStateRoot     *HexRoot            `json:"latestBlockHeaderStateRoot"`
	LatestBlockHeaderBodyRoot      *HexRoot            `json:"latestBlockHeaderBodyRoot"`
	HistoricalBlockHashesCount     *uint64             `json:"historicalBlockHashesCount"`
	HistoricalBlockHashes          *Container[HexRoot] `json:"historicalBlockHashes"`
	JustifiedSlots                 *Container[uint64]  `json:"justifiedSlots"`
	JustificationsRoots            *Container[HexRoot] `json:"justificationsRoots"`
	JustificationsValidators       *Container[bool]    `json:"justificationsValidators"`
}

// --- Fork Choice fixture types ---

// ForkChoiceFixture is the root JSON object: test_name -> test case.
type ForkChoiceFixture map[string]ForkChoiceTestCase

type ForkChoiceTestCase struct {
	Network     string           `json:"network"`
	AnchorState FixtureState     `json:"anchorState"`
	AnchorBlock FixtureBlock     `json:"anchorBlock"`
	Steps       []ForkChoiceStep `json:"steps"`
	MaxSlot     uint64           `json:"maxSlot"`
	Info        FixtureInfo      `json:"_info"`
}

type ForkChoiceStep struct {
	StepType    string                    `json:"stepType"`
	Valid       bool                      `json:"valid"`
	Checks      *StoreChecks              `json:"checks"`
	Block       *BlockStepData            `json:"block"`
	Time        *uint64                   `json:"time"`
	Attestation *FixtureSignedAttestation `json:"attestation"`
}

type BlockStepData struct {
	Block               FixtureBlock        `json:"block"`
	ProposerAttestation *FixtureAttestation `json:"proposerAttestation"`
}

// StoreChecks contains optional expected fields for selective fork choice validation.
type StoreChecks struct {
	Time                     *uint64            `json:"time"`
	HeadSlot                 *uint64            `json:"headSlot"`
	HeadRoot                 *HexRoot           `json:"headRoot"`
	HeadRootLabel            *string            `json:"headRootLabel"`
	LatestJustifiedSlot      *uint64            `json:"latestJustifiedSlot"`
	LatestJustifiedRoot      *HexRoot           `json:"latestJustifiedRoot"`
	LatestJustifiedRootLabel *string            `json:"latestJustifiedRootLabel"`
	LatestFinalizedSlot      *uint64            `json:"latestFinalizedSlot"`
	LatestFinalizedRoot      *HexRoot           `json:"latestFinalizedRoot"`
	LatestFinalizedRootLabel *string            `json:"latestFinalizedRootLabel"`
	AttestationChecks        []AttestationCheck `json:"attestationChecks"`
	LexicographicHeadAmong   []string           `json:"lexicographicHeadAmong"`
}

type AttestationCheck struct {
	Validator       uint64  `json:"validator"`
	AttestationSlot *uint64 `json:"attestationSlot"`
	HeadSlot        *uint64 `json:"headSlot"`
	SourceSlot      *uint64 `json:"sourceSlot"`
	TargetSlot      *uint64 `json:"targetSlot"`
	Location        string  `json:"location"` // "new" or "known"
}
