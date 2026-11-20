package attestation

import (
	"github.com/geanlabs/gean/internal/core/crypto/xmss"
	"github.com/geanlabs/gean/internal/lean/types"
)

// Lean's SSZ types store XMSS keys and signatures in fixed-size fields, sized
// by lean/types constants. Those must equal the XMSS scheme's own sizes. Each
// pair below fails to compile ("array bound must be non-negative") if the two
// differ in either direction.
var (
	_ [types.PubkeySize - xmss.PubkeySize]struct{}
	_ [xmss.PubkeySize - types.PubkeySize]struct{}

	_ [types.SignatureSize - xmss.SignatureSize]struct{}
	_ [xmss.SignatureSize - types.SignatureSize]struct{}
)
