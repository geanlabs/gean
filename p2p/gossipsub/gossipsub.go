// Package gossipsub implements gossipsub message handling for the Lean protocol.
package gossipsub

import (
	"github.com/devylongs/gean/types"
)

// Params holds the canonical gossipsub parameters.
type Params struct {
	ProtocolID        string
	D                 int     // Target mesh peers
	DLow              int     // Low watermark
	DHigh             int     // High watermark
	DLazy             int     // Gossip-only peers
	HeartbeatInterval float64 // Seconds
	FanoutTTL         int     // Seconds
	MCacheLen         int     // Message cache windows
	MCacheGossip      int     // Gossip windows
	SeenTTL           int     // Seen message TTL (seconds)
	ValidationMode    string
}

// Domain types for message ID isolation (per networking spec)
var (
	MessageDomainInvalidSnappy = [4]byte{0x00, 0x00, 0x00, 0x00}
	MessageDomainValidSnappy   = [4]byte{0x01, 0x00, 0x00, 0x00}
)

// DefaultParams returns the default gossipsub parameters for Devnet 0.
func DefaultParams() Params {
	// SeenTTL = SECONDS_PER_SLOT * JUSTIFICATION_LOOKBACK_SLOTS * 2
	// For Devnet 0: 4 * 3 * 2 = 24 seconds
	seenTTL := int(types.SecondsPerSlot) * int(types.JustificationLookbackSlots) * 2

	return Params{
		ProtocolID:        "/meshsub/1.0.0",
		D:                 8,
		DLow:              6,
		DHigh:             12,
		DLazy:             6,
		HeartbeatInterval: 0.7,
		FanoutTTL:         60,
		MCacheLen:         6,
		MCacheGossip:      3,
		SeenTTL:           seenTTL,
		ValidationMode:    "strict_no_sign",
	}
}

