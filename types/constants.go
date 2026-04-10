package types

const (
	// Timing
	SecondsPerSlot          = 4
	IntervalsPerSlot        = 5
	MillisecondsPerSlot     = SecondsPerSlot * 1000
	MillisecondsPerInterval = MillisecondsPerSlot / IntervalsPerSlot // 800

	// Limits
	HistoricalRootsLimit       = 1 << 18 // 262144
	ValidatorRegistryLimit     = 1 << 12 // 4096
	AttestationCommitteeCount  = 1
	JustificationLookbackSlots = 3
	MaxAttestationsData        = 16 // Maximum distinct AttestationData entries per block (leanSpec PR #536)

	// Derived limits
	JustificationValidatorsLimit = HistoricalRootsLimit * ValidatorRegistryLimit // 1073741824

	// Byte sizes
	PubkeySize     = 52
	SignatureSize  = 3112
	RootSize       = 32
	ByteListMiBMax = 1 << 20 // 1048576

	// Sync
	SyncToleranceSlots = 2
)
