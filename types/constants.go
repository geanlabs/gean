package types

// Protocol constants from the reference spec.
const (
	SecondsPerSlot          = 4
	IntervalsPerSlot        = 5
	MillisecondsPerSlot     = SecondsPerSlot * 1000
	MillisecondsPerInterval = MillisecondsPerSlot / IntervalsPerSlot // 800
	JustificationLookback   = 3
	MaxRequestBlocks        = 1024
	SlotsPerEpoch           = 32
)

// ZeroHash is a 32-byte zero hash used as genesis parent and padding.
var ZeroHash [32]byte
