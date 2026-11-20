package types

// Lean consensus constants. Unless marked otherwise, each is fixed by leanSpec
// (the lstar fork's config.py, or node/networking/config.py for networking)
// and is the same for every lean client, so they are constants rather than
// runtime configuration.

// Timing.
const (
	SecondsPerSlot          = 4
	IntervalsPerSlot        = 5
	MillisecondsPerSlot     = SecondsPerSlot * 1000
	MillisecondsPerInterval = MillisecondsPerSlot / IntervalsPerSlot
)

// SSZ sizes and limits. The generated SSZ code bakes these in through struct
// tags (ssz-size, ssz-max), so they must stay compile-time constants.
const (
	HistoricalRootsLimit         = 1 << 18
	ValidatorRegistryLimit       = 1 << 12
	JustificationValidatorsLimit = HistoricalRootsLimit * ValidatorRegistryLimit

	// PubkeySize and SignatureSize are the XMSS wire sizes. They must equal
	// xmss.PubkeySize and xmss.SignatureSize; lean/attestation checks this at
	// compile time.
	PubkeySize    = 32
	SignatureSize = 1208

	RootSize          = 32
	ByteList512KiBMax = 1 << 19
)

// Consensus protocol.
const (
	// AttestationCommitteeCount is the default number of attestation subnets.
	// It is a network parameter, not a fixed invariant: config.yaml's
	// ATTESTATION_COMMITTEE_COUNT or --attestation-committee-count override it.
	AttestationCommitteeCount  = 1
	JustificationLookbackSlots = 3
	MaxAttestationsData        = 8
	GossipDisparityIntervals   = 1
)

// Networking.
const (
	MaxRequestBlocks         = 1 << 10
	MinSlotsForBlockRequests = 3600
)
