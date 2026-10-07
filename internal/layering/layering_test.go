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
	libp2p       = "github.com/libp2p/go-libp2p"
	pebble       = "github.com/cockroachdb/pebble"
	pebbleDB     = module + "db/pebbledb"
	engine       = module + "node"
	p2p          = module + "p2p"
	syncer       = module + "syncer"
	api          = module + "api"
)

// rules maps each package to the dependencies it must never reach, directly or
// transitively. A dependency matches by import-path prefix.
var rules = map[string][]string{
	// Consensus logic: pure, so it builds and runs without native crypto, a
	// network, or a database engine.
	"types":            {module},
	"statetransition":  {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"forkchoice":       {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"attestationproof": {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"blockbuilder":     {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"genesis":          {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"pending":          {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"dutygate":         {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"proving":          {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"role":             {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"tasks":            {module},
	// The signature scheme contract depends only on consensus types.
	"crypto": {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api, module + "store"},
	// Storage: the consensus store is independent of the database engine.
	"db":    {nativeCrypto, fakeCrypto, libp2p, pebble, engine, module + "store"},
	"store": {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	// Signature-checking consensus steps use the crypto contract, never a
	// particular scheme, and never reach the network or the engine.
	"aggregation":    {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"attestation":    {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"blockprocessor": {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	// The engine depends on node.Network, db and the crypto contract, never on
	// an implementation.
	"node": {nativeCrypto, fakeCrypto, libp2p, pebble, p2p, syncer, api},
	// The simulation picks its scheme; the core never links native crypto.
	"sim": {nativeCrypto, libp2p, pebble, p2p, syncer, api},
	// Networking and serving sit beside the engine, not on top of it.
	"p2p":            {nativeCrypto, fakeCrypto, pebble, engine, syncer, api, module + "store"},
	"syncer":         {nativeCrypto, fakeCrypto, pebble, engine, api},
	"api":            {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer},
	"api/testdriver": {nativeCrypto, fakeCrypto, libp2p, pebble, engine, p2p, syncer},
	// The binaries never run on the forgeable scheme.
	"cmd/gean":   {fakeCrypto},
	"cmd/keygen": {fakeCrypto},
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
