//go:build spectests

package spectests

import "testing"

// schemaFixturesPending marks the fixture families whose bytes and roots encode
// the BlockBody and State shapes from before the execution payload was added.
// The pinned leanSpec has no execution payload, so its fixtures serialize a
// body without one and a state without the cached header; every state root,
// body root, and signed block root they carry is therefore computed on a shape
// gean no longer has.
//
// Families that do not touch those two containers (justifiability, slot clock,
// poseidon, networking codecs, single-message proofs, XMSS containers) keep
// running. Clear this once fixtures are regenerated from a spec that carries
// the payload.
const schemaFixturesPending = true

// schemaBoundSSZTypes are the SSZ fixture type names whose encoding changed.
var schemaBoundSSZTypes = map[string]bool{
	"BlockBody":   true,
	"Block":       true,
	"SignedBlock": true,
	"State":       true,
}

func skipIfSchemaFixturesPending(t *testing.T, family string) {
	t.Helper()
	if schemaFixturesPending {
		t.Skipf("%s fixtures predate the execution payload schema; regenerate from a spec that carries it", family)
	}
}
