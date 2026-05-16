// Package specfixtures contains the JSON-tagged fixture types that the hive
// lean simulator sends to a client's test-driver endpoints. The types mirror
// the Python leanSpec consensus fixture format and are also the format the
// internal go-side spec test runners under spectests/ already consume; they
// are factored out here so production code (the test-driver HTTP handlers)
// can decode requests without a test-only build tag.
package specfixtures

import "encoding/json"

// StateTransitionFixture is the top-level shape for a state-transition test
// case as the simulator delivers it on
// POST /lean/v0/test_driver/state_transition/run.
type StateTransitionFixture struct {
	Network                string         `json:"network"`
	LeanEnv                string         `json:"leanEnv"`
	Pre                    TestState      `json:"pre"`
	Blocks                 []TestBlock    `json:"blocks"`
	Post                   *TestPostState `json:"post"`
	ExpectException        string         `json:"expectException"`
	ExpectExceptionMessage string         `json:"expectExceptionMessage"`
}

// TestState carries a serialized lean consensus state in the form the
// simulator and leanSpec fixtures both use (camelCase JSON, validators in a
// {data: [...]} wrapper).
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

// TestConfig is the embedded ChainConfig portion of a fixture state.
type TestConfig struct {
	GenesisTime uint64 `json:"genesisTime"`
}

// TestBlockHeader is the hex-string-encoded BlockHeader on the wire.
type TestBlockHeader struct {
	Slot          uint64 `json:"slot"`
	ProposerIndex uint64 `json:"proposerIndex"`
	ParentRoot    string `json:"parentRoot"`
	StateRoot     string `json:"stateRoot"`
	BodyRoot      string `json:"bodyRoot"`
}

// TestCheckpoint is a {root: hex, slot: uint64} pair as sent on the wire.
type TestCheckpoint struct {
	Root string `json:"root"`
	Slot uint64 `json:"slot"`
}

// TestBlock is a fixture Block. State root is sent so the simulator can pin
// it; the handler reuses the parsed value verbatim rather than recomputing.
type TestBlock struct {
	Slot          uint64        `json:"slot"`
	ProposerIndex uint64        `json:"proposerIndex"`
	ParentRoot    string        `json:"parentRoot"`
	StateRoot     string        `json:"stateRoot"`
	Body          TestBlockBody `json:"body"`
	// BlockRootLabel is an optional fork-choice-fixture-only field used to tag
	// a produced root for later checks-step references. Ignored elsewhere.
	BlockRootLabel string `json:"blockRootLabel,omitempty"`
}

// TestBlockBody is the BlockBody portion — attestations only at this layer.
type TestBlockBody struct {
	Attestations TestDataList `json:"attestations"`
}

// TestDataList wraps the {"data": [...]} indirection the simulator uses for
// every SSZ list (validators, historicalBlockHashes, bitfields).
type TestDataList struct {
	Data []json.RawMessage `json:"data"`
}

// TestValidator is the per-validator entry. Dual pubkeys per devnet-4; the
// legacy single-Pubkey field is accepted for older fixtures that pre-date
// the proposer/attester key split.
type TestValidator struct {
	AttestationPubkey string `json:"attestationPubkey"`
	ProposalPubkey    string `json:"proposalPubkey"`
	Pubkey            string `json:"pubkey"`
	Index             uint64 `json:"index"`
}

// TestValidatorList wraps the validators array in its {"data": [...]} form.
type TestValidatorList struct {
	Data []TestValidator `json:"data"`
}

// TestAggregatedAttestation is a single aggregated attestation entry inside a
// block body or attestation-step payload.
type TestAggregatedAttestation struct {
	AggregationBits TestDataList `json:"aggregationBits"`
	Data            TestAttData  `json:"data"`
}

// TestAttData is the inner AttestationData record.
type TestAttData struct {
	Slot   uint64         `json:"slot"`
	Head   TestCheckpoint `json:"head"`
	Target TestCheckpoint `json:"target"`
	Source TestCheckpoint `json:"source"`
}

// TestPostState is the optional expected post-state for state-transition
// tests. Every field is a pointer so the simulator can pin a subset of the
// state without forcing a full deep comparison.
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

// ForkChoiceInit is the body of POST /lean/v0/test_driver/fork_choice/init.
type ForkChoiceInit struct {
	AnchorState TestState `json:"anchorState"`
	AnchorBlock TestBlock `json:"anchorBlock"`
	GenesisTime *uint64   `json:"genesisTime"`
}

// ForkChoiceStep is the body of POST /lean/v0/test_driver/fork_choice/step.
// StepType discriminates the active payload field.
type ForkChoiceStep struct {
	StepType    string               `json:"stepType"`
	Valid       bool                 `json:"valid"`
	Block       *TestBlock           `json:"block,omitempty"`
	Attestation *FCGossipAttestation `json:"attestation,omitempty"`
	Checks      *FCChecks            `json:"checks,omitempty"`
	Time        *uint64              `json:"time,omitempty"`
	Interval    *uint64              `json:"interval,omitempty"`
	HasProposal *bool                `json:"hasProposal,omitempty"`
}

// FCGossipAttestation carries either an individual signed attestation or an
// aggregated proof, depending on the step type.
type FCGossipAttestation struct {
	ValidatorID uint64      `json:"validatorId"`
	Data        TestAttData `json:"data"`
	Signature   string      `json:"signature"`
	Proof       *FCProof    `json:"proof,omitempty"`
}

// FCProof is an aggregated-signature proof payload from the gossip pipeline.
type FCProof struct {
	Participants TestDataList `json:"participants"`
	ProofData    FCProofData  `json:"proofData"`
}

// FCProofData wraps the hex-encoded proof bytes in the {"data": "0x..."} form
// the simulator emits.
type FCProofData struct {
	Data string `json:"data"`
}

// FCChecks lists post-step invariants the simulator will assert against the
// returned snapshot. Handlers do not interpret these; they're echoed back via
// the snapshot fields and asserted simulator-side.
type FCChecks struct {
	Time                     *uint64              `json:"time,omitempty"`
	HeadSlot                 *uint64              `json:"headSlot,omitempty"`
	HeadRoot                 *string              `json:"headRoot,omitempty"`
	HeadRootLabel            *string              `json:"headRootLabel,omitempty"`
	LatestJustifiedSlot      *uint64              `json:"latestJustifiedSlot,omitempty"`
	LatestJustifiedRoot      *string              `json:"latestJustifiedRoot,omitempty"`
	LatestJustifiedRootLabel *string              `json:"latestJustifiedRootLabel,omitempty"`
	LatestFinalizedSlot      *uint64              `json:"latestFinalizedSlot,omitempty"`
	LatestFinalizedRoot      *string              `json:"latestFinalizedRoot,omitempty"`
	LatestFinalizedRootLabel *string              `json:"latestFinalizedRootLabel,omitempty"`
	SafeTarget               *string              `json:"safeTarget,omitempty"`
	SafeTargetSlot           *uint64              `json:"safeTargetSlot,omitempty"`
	SafeTargetRootLabel      *string              `json:"safeTargetRootLabel,omitempty"`
	AttestationTargetSlot    *uint64              `json:"attestationTargetSlot,omitempty"`
	AttestationChecks        []FCAttestationCheck `json:"attestationChecks,omitempty"`
	LexicographicHeadAmong   []string             `json:"lexicographicHeadAmong,omitempty"`
}

// FCAttestationCheck pins one validator's latest attestation observation in
// a specific store location ("known" or "new").
type FCAttestationCheck struct {
	Validator       uint64  `json:"validator"`
	AttestationSlot *uint64 `json:"attestationSlot,omitempty"`
	HeadSlot        *uint64 `json:"headSlot,omitempty"`
	SourceSlot      *uint64 `json:"sourceSlot,omitempty"`
	TargetSlot      *uint64 `json:"targetSlot,omitempty"`
	Location        string  `json:"location"`
}

// VerifySignaturesFixture is the body of
// POST /lean/v0/test_driver/verify_signatures/run.
type VerifySignaturesFixture struct {
	Network                    string             `json:"network"`
	LeanEnv                    string             `json:"leanEnv"`
	AnchorState                TestState          `json:"anchorState"`
	SignedBlock                FixtureSignedBlock `json:"signedBlock"`
	SignedBlockWithAttestation FixtureSignedBlock `json:"signedBlockWithAttestation"`
	ExpectException            *string            `json:"expectException"`
}

// FixtureSignedBlock is the on-wire SignedBlock envelope. Supports both the
// devnet-4 flat form (block + signature siblings) and the legacy nested form
// (message wrapping the block) for forward compatibility with older fixtures.
type FixtureSignedBlock struct {
	Block     TestBlock          `json:"block"`
	Signature FixtureBlockSigPL  `json:"signature"`
	Message   *FixtureSBAMessage `json:"message,omitempty"`
}

// FixtureSBAMessage is the legacy SignedBlockWithAttestation wrapper.
type FixtureSBAMessage struct {
	Block TestBlock `json:"block"`
}

// FixtureBlockSigPL is the block signature payload: proposer signature plus
// per-attestation aggregated proofs.
type FixtureBlockSigPL struct {
	ProposerSignature     string             `json:"proposerSignature"`
	AttestationSignatures FixtureAttSigsList `json:"attestationSignatures"`
}

// FixtureAttSigsList wraps the per-attestation proofs in {"data": [...]}.
type FixtureAttSigsList struct {
	Data []FCProof `json:"data"`
}
