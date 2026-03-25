package node

import (
	"log/slog"
	"os"
	"testing"

	"github.com/geanlabs/gean/storage/memory"
	"github.com/geanlabs/gean/types"
)

func testBlock(slot uint64, parentRoot [32]byte) *types.SignedBlockWithAttestation {
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

func TestPendingBlockCache_AddAndHas(t *testing.T) {
	store := memory.New()
	cache := NewPendingBlockCache(store)

	parent := [32]byte{0xaa}
	sb := testBlock(5, parent)

	cache.Add(sb)

	root, _ := sb.Message.Block.HashTreeRoot()
	if !cache.Has(root) {
		t.Fatal("expected block to be in cache")
	}
	if cache.Len() != 1 {
		t.Fatalf("len = %d, want 1", cache.Len())
	}

	// Should also be persisted to store.
	if _, ok := store.GetPendingBlock(root); !ok {
		t.Fatal("expected block to be persisted in store")
	}
}

func TestPendingBlockCache_Remove(t *testing.T) {
	store := memory.New()
	cache := NewPendingBlockCache(store)

	parent := [32]byte{0xaa}
	sb := testBlock(5, parent)
	cache.Add(sb)

	root, _ := sb.Message.Block.HashTreeRoot()
	cache.Remove(root)

	if cache.Has(root) {
		t.Fatal("expected block to be removed from cache")
	}
	if cache.Len() != 0 {
		t.Fatalf("len = %d, want 0", cache.Len())
	}

	// Should also be removed from store.
	if _, ok := store.GetPendingBlock(root); ok {
		t.Fatal("expected block to be deleted from store")
	}
}

func TestPendingBlockCache_GetChildrenOf(t *testing.T) {
	store := memory.New()
	cache := NewPendingBlockCache(store)

	parent := [32]byte{0xaa}
	child1 := testBlock(5, parent)
	child2 := testBlock(6, parent)

	cache.Add(child1)
	cache.Add(child2)

	children := cache.GetChildrenOf(parent)
	if len(children) != 2 {
		t.Fatalf("children count = %d, want 2", len(children))
	}
}

func TestPendingBlockCache_PruneFinalized(t *testing.T) {
	store := memory.New()
	cache := NewPendingBlockCache(store)

	parent := [32]byte{0xaa}
	old := testBlock(5, parent)
	current := testBlock(10, parent)
	future := testBlock(15, parent)

	cache.Add(old)
	cache.Add(current)
	cache.Add(future)

	if cache.Len() != 3 {
		t.Fatalf("len = %d, want 3", cache.Len())
	}

	// Prune everything at or before slot 10.
	pruned := cache.PruneFinalized(10)
	if pruned != 2 {
		t.Fatalf("pruned = %d, want 2", pruned)
	}
	if cache.Len() != 1 {
		t.Fatalf("len after prune = %d, want 1", cache.Len())
	}

	// Verify future block is still there.
	futureRoot, _ := future.Message.Block.HashTreeRoot()
	if !cache.Has(futureRoot) {
		t.Fatal("expected future block to remain in cache")
	}

	// Verify pruned blocks are deleted from store.
	oldRoot, _ := old.Message.Block.HashTreeRoot()
	if _, ok := store.GetPendingBlock(oldRoot); ok {
		t.Fatal("expected pruned block to be deleted from store")
	}
	currentRoot, _ := current.Message.Block.HashTreeRoot()
	if _, ok := store.GetPendingBlock(currentRoot); ok {
		t.Fatal("expected pruned block to be deleted from store")
	}

	// Verify parent index is cleaned up.
	children := cache.GetChildrenOf(parent)
	if len(children) != 1 {
		t.Fatalf("children after prune = %d, want 1", len(children))
	}
}

func TestPendingBlockCache_LoadFromDB(t *testing.T) {
	store := memory.New()

	parent := [32]byte{0xaa}
	sb := testBlock(5, parent)
	root, _ := sb.Message.Block.HashTreeRoot()

	// Manually persist a pending block (simulates previous session).
	store.PutPendingBlock(root, sb)

	// Create a fresh cache and load from DB.
	cache := NewPendingBlockCache(store)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	cache.LoadFromDB(log)

	if !cache.Has(root) {
		t.Fatal("expected block to be loaded from DB")
	}
	if cache.Len() != 1 {
		t.Fatalf("len = %d, want 1", cache.Len())
	}

	// Children index should also be restored.
	children := cache.GetChildrenOf(parent)
	if len(children) != 1 {
		t.Fatalf("children = %d, want 1", len(children))
	}
}

func TestPendingBlockCache_DuplicateAdd(t *testing.T) {
	store := memory.New()
	cache := NewPendingBlockCache(store)

	parent := [32]byte{0xaa}
	sb := testBlock(5, parent)

	cache.Add(sb)
	cache.Add(sb) // duplicate

	if cache.Len() != 1 {
		t.Fatalf("len = %d, want 1 (duplicate should be ignored)", cache.Len())
	}
}

func TestPendingBlockCache_NilBlock(t *testing.T) {
	store := memory.New()
	cache := NewPendingBlockCache(store)

	cache.Add(nil)
	if cache.Len() != 0 {
		t.Fatalf("len = %d, want 0 (nil should be ignored)", cache.Len())
	}
}
