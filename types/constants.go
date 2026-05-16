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
	MaxAttestationsData        = 16

	// Derived limits
	JustificationValidatorsLimit = HistoricalRootsLimit * ValidatorRegistryLimit // 1073741824

	// Byte sizes
	PubkeySize     = 52
	SignatureSize  = 2536
	RootSize       = 32
	ByteListMiBMax = 1 << 20 // 1048576

	// Sync
	SyncToleranceSlots = 2

	// GossipDisparityIntervals bounds the clock skew the time check is willing
	// to absorb when admitting a vote whose slot has not yet started locally.
	// One interval is roughly 800ms, the lean analogue of mainnet's
	// MAXIMUM_GOSSIP_CLOCK_DISPARITY. Per leanSpec PR #682.
	GossipDisparityIntervals = 1

	// Networking — req/resp protocol limits.

	// MaxRequestBlocks caps the number of blocks a single BlocksByRange
	// request may ask for. Per leanSpec PR #691 (subspecs/networking/config.py).
	MaxRequestBlocks = 1 << 10 // 1024

	// MinSlotsForBlockRequests is the sliding-window history depth a server
	// must serve. Requests whose start_slot falls below
	// (current_slot - MinSlotsForBlockRequests) receive RESOURCE_UNAVAILABLE.
	// Per leanSpec PR #691 (subspecs/networking/config.py).
	MinSlotsForBlockRequests = 3600
)
