package xmss

// Wire sizes of the XMSS scheme gean signs and verifies with (leanVM). Code
// that stores keys or signatures in fixed-size fields, such as lean's SSZ
// types, must use the same sizes; lean checks this at compile time.
const (
	// PubkeySize is the length of a serialized XMSS public key, in bytes.
	PubkeySize = 32
	// SignatureSize is the length of a serialized XMSS signature, in bytes.
	SignatureSize = 1208
)
