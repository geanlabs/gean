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
	"statetransition":  {nativeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"forkchoice":       {nativeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"attestationproof": {nativeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"blockbuilder":     {nativeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"genesis":          {nativeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"pending":          {nativeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"dutygate":         {nativeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"proving":          {nativeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"role":             {nativeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	"tasks":            {module},
	// Storage: the consensus store is independent of the database engine.
	"db":    {nativeCrypto, libp2p, pebble, engine, module + "store"},
	"store": {nativeCrypto, libp2p, pebble, engine, p2p, syncer, api},
	// Crypto-backed consensus steps never reach the network or the engine.
	"aggregation":    {libp2p, pebble, engine, p2p, syncer, api},
	"attestation":    {libp2p, pebble, engine, p2p, syncer, api},
	"blockprocessor": {libp2p, pebble, engine, p2p, syncer, api},
	// The engine depends on node.Network and db, never on an implementation.
	"node": {libp2p, pebble, p2p, syncer, api},
	"sim":  {libp2p, pebble, p2p, syncer, api},
	// Networking and serving sit beside the engine, not on top of it.
	"p2p":    {nativeCrypto, pebble, engine, syncer, api, module + "store"},
	"syncer": {nativeCrypto, pebble, engine, api},
	"api":    {libp2p, pebble, engine, p2p, syncer},
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
