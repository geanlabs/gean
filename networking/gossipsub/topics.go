// topics.go contains topic constants for Lean gossipsub traffic.
package gossipsub

const NetworkName = "devnet0"

// Topic format: /leanconsensus/<network>/<type>/ssz_snappy
// NetworkName stays "devnet0" — all interop clients use this regardless of version.
var (
	BlockTopic       = "/leanconsensus/" + NetworkName + "/block/ssz_snappy"
	AttestationTopic = "/leanconsensus/" + NetworkName + "/attestation/ssz_snappy"
)
