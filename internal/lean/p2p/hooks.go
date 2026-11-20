package p2p

// GossipHooks observe the size of each decompressed gossip message, by kind.
// Each may be nil.
type GossipHooks struct {
	BlockSize       func(bytes int)
	AttestationSize func(bytes int)
	AggregationSize func(bytes int)
}
