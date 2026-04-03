package node

import (
	"testing"

	"github.com/geanlabs/gean/forkchoice"
	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
)

func makeTestEngine() *Engine {
	backend := storage.NewInMemoryBackend()
	s := NewConsensusStore(backend)

	// Set up genesis state.
	s.SetConfig(&types.ChainConfig{GenesisTime: 1000})
	var genesisRoot [32]byte
	genesisRoot[0] = 0x01
	s.SetHead(genesisRoot)
	s.SetSafeTarget(genesisRoot)
	s.SetLatestJustified(&types.Checkpoint{Root: genesisRoot, Slot: 0})
	s.SetLatestFinalized(&types.Checkpoint{Root: genesisRoot, Slot: 0})
	s.InsertBlockHeader(genesisRoot, &types.BlockHeader{Slot: 0})

	genesisState := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     0,
		LatestBlockHeader:        &types.BlockHeader{},
		LatestJustified:          &types.Checkpoint{Root: genesisRoot, Slot: 0},
		LatestFinalized:          &types.Checkpoint{Root: genesisRoot, Slot: 0},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
	s.InsertState(genesisRoot, genesisState)

	fc := forkchoice.New(0, genesisRoot)

	return New(s, fc, nil, nil, false, 1)
}

func TestEngineCreation(t *testing.T) {
	e := makeTestEngine()
	if e.Store == nil {
		t.Fatal("store should not be nil")
	}
	if e.FC == nil {
		t.Fatal("fork choice should not be nil")
	}
}

func TestEngineUpdateHead(t *testing.T) {
	e := makeTestEngine()
	e.updateHead(false)

	head := e.Store.Head()
	if types.IsZeroRoot(head) {
		t.Fatal("head should not be zero after updateHead")
	}
}

func TestEngineUpdateSafeTarget(t *testing.T) {
	e := makeTestEngine()
	e.updateSafeTarget()

	safeTarget := e.Store.SafeTarget()
	if types.IsZeroRoot(safeTarget) {
		t.Fatal("safe target should not be zero")
	}
}

func TestEnginePendingBlocks(t *testing.T) {
	e := makeTestEngine()

	var blockRoot, parentRoot [32]byte
	blockRoot[0] = 0x10
	parentRoot[0] = 0x20

	// Manually add pending entries (simulates addPendingBlock logic).
	e.PendingBlockParents[blockRoot] = parentRoot
	children := make(map[[32]byte]bool)
	children[blockRoot] = true
	e.PendingBlocks[parentRoot] = children

	if len(e.PendingBlocks) != 1 {
		t.Fatalf("expected 1 pending parent, got %d", len(e.PendingBlocks))
	}
	if len(e.PendingBlockParents) != 1 {
		t.Fatalf("expected 1 pending block, got %d", len(e.PendingBlockParents))
	}
}

func TestEngineCascadePending(t *testing.T) {
	e := makeTestEngine()

	var parentRoot, child1, child2 [32]byte
	parentRoot[0] = 0x01
	child1[0] = 0x10
	child2[0] = 0x20

	e.PendingBlockParents[child1] = parentRoot
	e.PendingBlockParents[child2] = parentRoot
	children := make(map[[32]byte]bool)
	children[child1] = true
	children[child2] = true
	e.PendingBlocks[parentRoot] = children

	if len(e.PendingBlocks[parentRoot]) != 2 {
		t.Fatalf("expected 2 children pending, got %d", len(e.PendingBlocks[parentRoot]))
	}

	// collectPendingChildren removes entries and returns blocks to process.
	var queue []*types.SignedBlockWithAttestation
	e.collectPendingChildren(parentRoot, &queue)

	if len(e.PendingBlocks) != 0 {
		t.Fatalf("expected 0 pending after cascade, got %d", len(e.PendingBlocks))
	}
	if len(e.PendingBlockParents) != 0 {
		t.Fatalf("expected 0 pending parents after cascade, got %d", len(e.PendingBlockParents))
	}
}

func TestEngineMessageHandler(t *testing.T) {
	e := makeTestEngine()

	// Verify Engine implements the MessageHandler interface.
	block := &types.SignedBlockWithAttestation{
		Block: &types.BlockWithAttestation{
			Block:               &types.Block{Slot: 1},
			ProposerAttestation: &types.Attestation{},
		},
		Signature: &types.BlockSignatures{},
	}

	// Should not panic — just push to channel.
	e.OnBlock(block)

	// Check channel received it.
	select {
	case received := <-e.BlockCh:
		if received.Block.Block.Slot != 1 {
			t.Fatal("wrong block slot")
		}
	default:
		t.Fatal("block should be in channel")
	}
}

func TestEngineGetOurProposer(t *testing.T) {
	e := makeTestEngine()
	// No keys — should return false.
	_, ok := e.getOurProposer(1)
	if ok {
		t.Fatal("should not be proposer without keys")
	}
}

func TestEngineCurrentSlot(t *testing.T) {
	e := makeTestEngine()
	// Genesis at 1000s, slot 1 starts at 1004s.
	slot := e.currentSlot(1004 * 1000) // 1004000ms
	if slot != 1 {
		t.Fatalf("expected slot 1, got %d", slot)
	}
}

func TestEngineCurrentInterval(t *testing.T) {
	e := makeTestEngine()
	// Genesis at 1000s. Interval 0 of slot 1 starts at 1004000ms.
	// Interval 1 starts at 1004800ms.
	interval := e.currentInterval(1004800)
	if interval != 1 {
		t.Fatalf("expected interval 1, got %d", interval)
	}
}
