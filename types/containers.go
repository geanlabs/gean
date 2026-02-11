package types

//go:generate go run github.com/ferranbt/fastssz/sszgen --path=. --objs=Checkpoint,Config,AttestationData,Attestation,SignedAttestation,Validator,BlockHeader,BlockBody,Block,BlockWithAttestation,SignedBlockWithAttestation,State

// SSZ containers for the Lean Ethereum consensus protocol.
// Field order is critical for SSZ serialization and must match the spec exactly.

// Checkpoint identifies a block at a specific slot in the chain.
// Used for justification and finalization tracking.
type Checkpoint struct {
	Root Root `ssz-size:"32"`
	Slot Slot
}

// Config holds immutable chain configuration parameters.
type Config struct {
	GenesisTime uint64
}

// AttestationData describes a validator's observed chain view.
type AttestationData struct {
	Slot   Slot
	Head   Checkpoint
	Target Checkpoint
	Source Checkpoint
}

// Attestation wraps attestation data with the validator's identity.
// Separated from AttestationData to enable aggregation — multiple validators
// can attest to the same data.
type Attestation struct {
	ValidatorID uint64
	Data        AttestationData
}

// SignedAttestation wraps an Attestation with its XMSS signature.
type SignedAttestation struct {
	Message   Attestation
	Signature Signature `ssz-size:"3112"`
}

// Validator represents a validator's identity in the state registry.
type Validator struct {
	Pubkey Pubkey `ssz-size:"52"`
	Index  ValidatorIndex
}

// BlockHeader is the fixed-size portion of a block, used for parent chain linking.
// The StateRoot is initially zero and filled during ProcessSlots before slot advance.
type BlockHeader struct {
	Slot          Slot
	ProposerIndex uint64
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	BodyRoot      Root `ssz-size:"32"`
}

// BlockBody contains the variable-length block contents.
// Attestations are unsigned here; signatures are in the SignedBlockWithAttestation envelope.
type BlockBody struct {
	Attestations []Attestation `ssz-max:"4096"`
}

// Block is a consensus block containing header fields and a body.
type Block struct {
	Slot          Slot
	ProposerIndex uint64
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	Body          BlockBody
}

// BlockWithAttestation bundles a block with the proposer's own attestation.
// The proposer attestation is separate from body attestations so it can be
// processed with different fork choice semantics.
type BlockWithAttestation struct {
	Block               Block
	ProposerAttestation Attestation
}

// SignedBlockWithAttestation is the top-level block envelope on the network.
// The Signature list contains one entry per body attestation, followed by the
// proposer's signature: [att_0_sig, ..., att_n_sig, proposer_sig].
type SignedBlockWithAttestation struct {
	Message   BlockWithAttestation
	Signature []Signature `ssz-max:"4096" ssz-size:"?,3112"`
}

// State is the full consensus state object.
// Field order must match the spec exactly for correct SSZ serialization.
type State struct {
	Config            Config
	Slot              Slot
	LatestBlockHeader BlockHeader

	LatestJustified Checkpoint
	LatestFinalized Checkpoint

	HistoricalBlockHashes   []Root      `ssz-max:"262144" ssz-size:"?,32"`   // List[Bytes32, HISTORICAL_ROOTS_LIMIT]
	JustifiedSlots          []byte      `ssz:"bitlist" ssz-max:"262144"`     // Bitlist[HISTORICAL_ROOTS_LIMIT]
	Validators              []Validator `ssz-max:"4096"`                     // List[Validator, VALIDATOR_REGISTRY_LIMIT]
	JustificationRoots      []Root      `ssz-max:"262144" ssz-size:"?,32"`   // List[Bytes32, HISTORICAL_ROOTS_LIMIT]
	JustificationValidators []byte      `ssz:"bitlist" ssz-max:"1073741824"` // Bitlist[HISTORICAL_ROOTS_LIMIT * VALIDATOR_REGISTRY_LIMIT]
}
