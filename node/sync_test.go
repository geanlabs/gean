package node

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/types"
)

// ---------------------------------------------------------------------------
// inflightRoots tests
// ---------------------------------------------------------------------------

func TestInflightRoots_Dedup(t *testing.T) {
	ir := newInflightRoots()
	root := [32]byte{1}

	if !ir.tryAcquire(root) {
		t.Fatal("first acquire should succeed")
	}
	if ir.tryAcquire(root) {
		t.Fatal("duplicate acquire should be rejected")
	}
}

func TestInflightRoots_ReleaseAllows(t *testing.T) {
	ir := newInflightRoots()
	root := [32]byte{2}

	ir.tryAcquire(root)
	ir.release(root)

	if !ir.tryAcquire(root) {
		t.Fatal("acquire after release should succeed")
	}
}

func TestInflightRoots_ReleaseStale(t *testing.T) {
	ir := newInflightRoots()
	root := [32]byte{3}

	ir.tryAcquire(root)
	// Backdate the entry.
	ir.mu.Lock()
	ir.roots[root] = time.Now().Add(-time.Minute)
	ir.mu.Unlock()

	ir.releaseStale(30 * time.Second)

	if !ir.tryAcquire(root) {
		t.Fatal("acquire after stale cleanup should succeed")
	}
}

func TestInflightRoots_ReleaseStaleKeepsFresh(t *testing.T) {
	ir := newInflightRoots()
	root := [32]byte{4}

	ir.tryAcquire(root)
	ir.releaseStale(30 * time.Second)

	if ir.tryAcquire(root) {
		t.Fatal("fresh entry should not be cleaned up")
	}
}

// ---------------------------------------------------------------------------
// peerLimiter tests
// ---------------------------------------------------------------------------

func TestPeerLimiter_MaxConcurrent(t *testing.T) {
	pl := newPeerLimiter()
	pid := peer.ID("test-peer")

	if !pl.acquire(pid) {
		t.Fatal("first acquire should succeed")
	}
	if !pl.acquire(pid) {
		t.Fatal("second acquire should succeed (limit is 2)")
	}
	if pl.acquire(pid) {
		t.Fatal("third acquire should fail (at limit)")
	}
}

func TestPeerLimiter_ReleaseReenables(t *testing.T) {
	pl := newPeerLimiter()
	pid := peer.ID("test-peer")

	pl.acquire(pid)
	pl.acquire(pid)
	pl.release(pid)

	if !pl.acquire(pid) {
		t.Fatal("acquire after release should succeed")
	}
}

func TestPeerLimiter_IndependentPeers(t *testing.T) {
	pl := newPeerLimiter()
	pid1 := peer.ID("peer-1")
	pid2 := peer.ID("peer-2")

	pl.acquire(pid1)
	pl.acquire(pid1)

	if !pl.acquire(pid2) {
		t.Fatal("different peer should not be affected by first peer's limit")
	}
}

// ---------------------------------------------------------------------------
// recoveryCoordinator tests
// ---------------------------------------------------------------------------

func TestRecoveryCoordinator_Dedup(t *testing.T) {
	rc := newRecoveryCoordinator()
	root := [32]byte{10}

	if !rc.tryStartRecovery(root) {
		t.Fatal("first recovery should start")
	}
	if rc.tryStartRecovery(root) {
		t.Fatal("duplicate recovery should be rejected")
	}
}

func TestRecoveryCoordinator_FinishAllowsRestart(t *testing.T) {
	rc := newRecoveryCoordinator()
	root := [32]byte{11}

	rc.tryStartRecovery(root)
	rc.finishRecovery(root)

	if !rc.tryStartRecovery(root) {
		t.Fatal("recovery after finish should start")
	}
}

func TestRecoveryCoordinator_Cooldown(t *testing.T) {
	rc := newRecoveryCoordinator()
	root := [32]byte{12}

	rc.tryStartRecovery(root)
	rc.finishRecovery(root)
	rc.setCooldown(root, 100*time.Millisecond)

	if rc.tryStartRecovery(root) {
		t.Fatal("recovery during cooldown should be rejected")
	}

	time.Sleep(150 * time.Millisecond)

	if !rc.tryStartRecovery(root) {
		t.Fatal("recovery after cooldown expiry should start")
	}
}

// ---------------------------------------------------------------------------
// PendingBlockCache.MissingParents tests
// ---------------------------------------------------------------------------

func makeTestBlock(slot uint64, parentRoot [32]byte) *types.SignedBlockWithAttestation {
	return &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{
			Block: &types.Block{
				Slot:       slot,
				ParentRoot: parentRoot,
				Body:       &types.BlockBody{},
			},
		},
	}
}

func TestPendingBlockCache_MissingParents(t *testing.T) {
	cache := NewPendingBlockCache()

	parentA := [32]byte{0xAA}
	parentB := [32]byte{0xBB}

	cache.Add(makeTestBlock(10, parentA))
	cache.Add(makeTestBlock(11, parentA)) // same parent
	cache.Add(makeTestBlock(12, parentB))

	missing := cache.MissingParents()
	if len(missing) != 2 {
		t.Fatalf("expected 2 missing parents, got %d", len(missing))
	}

	roots := make(map[[32]byte]bool)
	for _, r := range missing {
		roots[r] = true
	}
	if !roots[parentA] || !roots[parentB] {
		t.Fatal("expected both parentA and parentB in missing parents")
	}
}

func TestPendingBlockCache_MissingParents_ExcludesCached(t *testing.T) {
	cache := NewPendingBlockCache()

	parentParent := [32]byte{0xDD}
	parentBlock := makeTestBlock(19, parentParent)
	parentRoot, err := parentBlock.Message.Block.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash parent block: %v", err)
	}
	childBlock := makeTestBlock(20, parentRoot)

	cache.Add(parentBlock)
	cache.Add(childBlock)

	missing := cache.MissingParents()
	if len(missing) != 1 {
		t.Fatalf("expected 1 missing parent, got %d", len(missing))
	}
	if missing[0] != parentParent {
		t.Fatalf("expected missing parent %x, got %x", parentParent, missing[0])
	}
}

// ---------------------------------------------------------------------------
// PendingBlockCache.PruneFinalized tests
// ---------------------------------------------------------------------------

func TestPendingBlockCache_PruneFinalized(t *testing.T) {
	cache := NewPendingBlockCache()

	cache.Add(makeTestBlock(5, [32]byte{1}))
	cache.Add(makeTestBlock(10, [32]byte{2}))
	cache.Add(makeTestBlock(15, [32]byte{3}))
	cache.Add(makeTestBlock(20, [32]byte{4}))

	pruned := cache.PruneFinalized(10)
	if pruned != 2 {
		t.Fatalf("expected 2 pruned (slots 5 and 10), got %d", pruned)
	}
	if cache.Len() != 2 {
		t.Fatalf("expected 2 remaining, got %d", cache.Len())
	}
}

func TestPendingBlockCache_PruneFinalized_Empty(t *testing.T) {
	cache := NewPendingBlockCache()
	pruned := cache.PruneFinalized(100)
	if pruned != 0 {
		t.Fatalf("expected 0 pruned on empty cache, got %d", pruned)
	}
}

func TestPendingBlockCache_PruneFinalized_IndexCoherence(t *testing.T) {
	cache := NewPendingBlockCache()

	parentRoot := [32]byte{0xEE}
	cache.Add(makeTestBlock(5, parentRoot))
	cache.Add(makeTestBlock(15, parentRoot))

	cache.PruneFinalized(10)

	// After pruning slot 5, only slot 15 should remain.
	children := cache.GetChildrenOf(parentRoot)
	if len(children) != 1 {
		t.Fatalf("expected 1 child after prune, got %d", len(children))
	}
	if children[0].Message.Block.Slot != 15 {
		t.Fatalf("expected remaining child at slot 15, got %d", children[0].Message.Block.Slot)
	}
}
