// Package layering enforces the dependency rules between gean's packages, the
// way prysm's Bazel visibility rules and reth's crate graph do: consensus logic
// stays free of native crypto, networking and the storage engine, and the
// engine stays free of any particular network or database.
package layering

import (
	"os/exec"
	"strings"
	"testing"
)

const module = "github.com/geanlabs/gean/"

const (
	nativeCrypto = module + "crypto/xmss"
	fakeCrypto   = module + "crypto/insecure"
	schemeTest   = module + "crypto/schemetest"
	simulation   = module + "sim"
	testDriver   = module + "api/testdriver"
	dbTest       = module + "storage/db/dbtest"
	libp2p       = "github.com/libp2p/go-libp2p"
	pebble       = "github.com/cockroachdb/pebble"
	pebbleDB     = module + "storage/db/pebbledb"
	engine       = module + "node"
	p2p          = module + "net/p2p"
	syncer       = module + "net/syncer"
	api          = module + "api"
)

// rules maps every package to the dependencies it must never reach, directly or
// transitively. A dependency matches by import-path prefix. A package without
// a rule fails the test, so a new package must declare its layer.
var rules = map[string][]string{
	// Consensus logic: pure, so it builds and runs without native crypto, a
	// network, or a database engine.
	"types":                      {module},
	"consensus/statetransition":  {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"consensus/forkchoice":       {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"consensus/attestationproof": {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"consensus/blockbuilder":     {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"consensus/genesis":          {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"pending":                    {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"dutygate":                   {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"proving":                    {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"role":                       {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"tasks":                      {module},
	"logger":                     {module},
	"metrics":                    {module},
	"shadow":                     {module},
	// The signature scheme contract depends only on consensus types; its
	// implementations and conformance suite never reach node code.
	"crypto":            {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api, module + "storage/store"},
	"crypto/xmss":       {fakeCrypto, libp2p, pebble, engine, p2p, syncer, api, module + "storage/store"},
	"crypto/insecure":   {nativeCrypto, libp2p, pebble, engine, p2p, syncer, api, module + "storage/store"},
	"crypto/schemetest": {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api, module + "storage/store"},
	"net/checkpoint":    {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api, module + "storage/store"},
	// Storage: the consensus store is independent of the database engine.
	"storage/db":          {nativeCrypto, fakeCrypto, libp2p, pebble, engine, module + "storage/store"},
	"storage/db/pebbledb": {nativeCrypto, fakeCrypto, libp2p, engine, module + "storage/store"},
	"storage/db/dbtest":   {nativeCrypto, fakeCrypto, libp2p, pebble, engine, module + "storage/store"},
	"storage/store":       {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	// Signature-checking consensus steps use the crypto contract, never a
	// particular scheme, and never reach the network or the engine.
	"consensus/aggregation":    {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"consensus/attestation":    {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"consensus/blockprocessor": {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	// The engine depends on node.Network, db and the crypto contract, never on
	// an implementation.
	"node": {nativeCrypto, fakeCrypto, libp2p, pebble, p2p, syncer, api},
	// The simulation picks its scheme; the core never links native crypto.
	"sim":         {nativeCrypto, libp2p, pebble, p2p, api},
	"sim/xmsssim": {libp2p, pebble, p2p, api},
	// Networking and serving sit beside the engine, not on top of it.
	"net/p2p":        {nativeCrypto, fakeCrypto, pebble, engine, syncer, api, module + "storage/store"},
	"net/syncer":     {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, api},
	"api":            {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer},
	"api/testdriver": {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer},
	// Production code never links test code: not the forgeable scheme, the
	// conformance suites, the simulation or the hive test driver.
	"launch":     {fakeCrypto, schemeTest, simulation, testDriver, dbTest},
	"cmd/gean":   {fakeCrypto, schemeTest, simulation, testDriver, dbTest},
	"cmd/keygen": {fakeCrypto, schemeTest, simulation, testDriver, dbTest},
	// Test infrastructure.
	"internal/layering":     {module},
	"internal/specfixtures": {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer},
}

func TestPackageLayering(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps=false", "-f", `{{.ImportPath}} {{join .Deps " "}}`, module+"...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	deps := make(map[string][]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		deps[strings.TrimPrefix(fields[0], module)] = fields[1:]
	}

	for pkg := range deps {
		if _, ok := rules[pkg]; !ok {
			t.Errorf("package %s has no layering rule; add one for its layer", pkg)
		}
	}
	for pkg, forbidden := range rules {
		pkgDeps, ok := deps[pkg]
		if !ok {
			t.Errorf("rule for %s matches no package", pkg)
			continue
		}
		for _, dep := range pkgDeps {
			for _, f := range forbidden {
				if dep == f || strings.HasPrefix(dep, f+"/") || (strings.HasSuffix(f, "/") && strings.HasPrefix(dep, f)) {
					t.Errorf("%s depends on %s, which its layer forbids", pkg, dep)
				}
			}
		}
	}
}
