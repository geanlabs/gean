// Package p2p implements networking for the Lean Ethereum consensus protocol.
package p2p

// Gossipsub topic names (Devnet 0, per networking spec)
const (
	TopicBlocks = "/leanconsensus/devnet0/block/ssz_snappy"
	TopicVotes  = "/leanconsensus/devnet0/vote/ssz_snappy"
)
