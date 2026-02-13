package types

// Protocol constants from the reference spec.
const (
	SecondsPerSlot         = 4
	IntervalsPerSlot       = 4
	SecondsPerInterval     = SecondsPerSlot / IntervalsPerSlot // 1
	JustificationLookback  = 3
	MaxRequestBlocks       = 1024
)

// ZeroHash is a 32-byte zero hash used as genesis parent and padding.
var ZeroHash [32]byte
