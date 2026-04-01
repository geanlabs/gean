package forkchoice

import (
	"testing"

	"github.com/geanlabs/gean/storage/memory"
	"github.com/geanlabs/gean/types"
)

// makeTestStore creates a minimal Store with n validators for pruning tests.
func makeTestStore(numValidators int) *Store {
	validators := make([]*types.Validator, numValidators)
	for i := range validators {
		validators[i] = &types.Validator{Index: uint64(i)}
	}
	genesisBlock := &types.Block{
		Slot: 0,
		Body: &types.BlockBody{},
	}
	genesisRoot, _ := genesisBlock.HashTreeRoot()

	state := &types.State{
		Config:                   &types.Config{GenesisTime: 1000},
		Slot:                     0,
		Validators:               validators,
		LatestBlockHeader:        &types.BlockHeader{},
		LatestJustified:          &types.Checkpoint{Root: genesisRoot, Slot: 0},
		LatestFinalized:          &types.Checkpoint{Root: genesisRoot, Slot: 0},
		JustifiedSlots:           []byte{0x01},
		JustificationsValidators: []byte{0x01},
	}

	store := memory.New()
	store.PutBlock(genesisRoot, genesisBlock)
	store.PutState(genesisRoot, state)

	return &Store{
		time:                          0,
		genesisTime:                   1000,
		numValidators:                 uint64(numValidators),
		head:                          genesisRoot,
		safeTarget:                    genesisRoot,
		latestJustified:               &types.Checkpoint{Root: genesisRoot, Slot: 0},
		latestFinalized:               &types.Checkpoint{Root: genesisRoot, Slot: 0},
		storage:                       store,
		latestKnownAttestations:       make(map[uint64]*types.SignedAttestation),
		latestNewAttestations:         make(map[uint64]*types.SignedAttestation),
		latestKnownAggregatedPayloads: make(map[[32]byte]aggregatedPayload),
		latestNewAggregatedPayloads:   make(map[[32]byte]aggregatedPayload),
		gossipSignatures:              make(map[signatureKey]storedSignature),
		aggregatedPayloads:            make(map[signatureKey][]storedAggregatedPayload),
	}
}

func makeAttestationData(slot uint64, targetSlot uint64) *types.AttestationData {
	return &types.AttestationData{
		Slot:   slot,
		Head:   &types.Checkpoint{Root: [32]byte{byte(slot)}, Slot: slot},
		Target: &types.Checkpoint{Root: [32]byte{byte(targetSlot)}, Slot: targetSlot},
		Source: &types.Checkpoint{Root: [32]byte{}, Slot: 0},
	}
}

func TestPruneStaleAttestationData(t *testing.T) {
	fc := makeTestStore(4)

	// Add payloads with various target slots.
	for i := uint64(1); i <= 5; i++ {
		data := makeAttestationData(i, i)
		key, _ := makeAttestationDataKey(data)
		fc.latestKnownAggregatedPayloads[key] = aggregatedPayload{data: data}
		fc.latestNewAggregatedPayloads[key] = aggregatedPayload{data: data}
	}

	if len(fc.latestKnownAggregatedPayloads) != 5 {
		t.Fatalf("expected 5 known payloads, got %d", len(fc.latestKnownAggregatedPayloads))
	}

	// Prune with finalized slot = 3. Targets 1,2,3 should be removed.
	fc.pruneStaleAttestationData(3)

	if len(fc.latestKnownAggregatedPayloads) != 2 {
		t.Fatalf("expected 2 known payloads after pruning, got %d", len(fc.latestKnownAggregatedPayloads))
	}
	if len(fc.latestNewAggregatedPayloads) != 2 {
		t.Fatalf("expected 2 new payloads after pruning, got %d", len(fc.latestNewAggregatedPayloads))
	}

	// Verify the remaining entries have target slots > 3.
	for _, payload := range fc.latestKnownAggregatedPayloads {
		if payload.data.Target.Slot <= 3 {
			t.Fatalf("expected remaining payload target > 3, got %d", payload.data.Target.Slot)
		}
	}
}

func TestPruneAggregatedPayloadsCache(t *testing.T) {
	fc := makeTestStore(4)

	// Add gossip signatures and aggregated payload cache entries at various slots.
	for i := uint64(1); i <= 6; i++ {
		data := makeAttestationData(i, i)
		key := signatureKey{validatorID: 0, dataRoot: [32]byte{byte(i)}}
		fc.gossipSignatures[key] = storedSignature{slot: i, data: data}
		fc.aggregatedPayloads[key] = []storedAggregatedPayload{
			{slot: i, proof: &types.AggregatedSignatureProof{Participants: []byte{0x03}, ProofData: []byte{byte(i)}}},
		}
	}

	if len(fc.gossipSignatures) != 6 {
		t.Fatalf("expected 6 gossip signatures, got %d", len(fc.gossipSignatures))
	}
	if len(fc.aggregatedPayloads) != 6 {
		t.Fatalf("expected 6 aggregated payload keys, got %d", len(fc.aggregatedPayloads))
	}

	// Prune with finalized slot = 4.
	fc.pruneAggregatedPayloadsCache(4)

	if len(fc.gossipSignatures) != 2 {
		t.Fatalf("expected 2 gossip signatures after pruning, got %d", len(fc.gossipSignatures))
	}
	if len(fc.aggregatedPayloads) != 2 {
		t.Fatalf("expected 2 aggregated payload keys after pruning, got %d", len(fc.aggregatedPayloads))
	}
}

func TestPruneStorage(t *testing.T) {
	fc := makeTestStore(4)

	// Build a chain: genesis -> slot 1 -> slot 2 -> slot 3 (head, finalized)
	genesisRoot := fc.head

	block1 := &types.Block{Slot: 1, ParentRoot: genesisRoot, Body: &types.BlockBody{}}
	root1, _ := block1.HashTreeRoot()
	fc.storage.PutBlock(root1, block1)
	fc.storage.PutSignedBlock(root1, &types.SignedBlockWithAttestation{Message: &types.BlockWithAttestation{Block: block1}})
	fc.storage.PutState(root1, &types.State{Slot: 1})

	block2 := &types.Block{Slot: 2, ParentRoot: root1, Body: &types.BlockBody{}}
	root2, _ := block2.HashTreeRoot()
	fc.storage.PutBlock(root2, block2)
	fc.storage.PutSignedBlock(root2, &types.SignedBlockWithAttestation{Message: &types.BlockWithAttestation{Block: block2}})
	fc.storage.PutState(root2, &types.State{Slot: 2})

	block3 := &types.Block{Slot: 3, ParentRoot: root2, Body: &types.BlockBody{}}
	root3, _ := block3.HashTreeRoot()
	fc.storage.PutBlock(root3, block3)
	fc.storage.PutSignedBlock(root3, &types.SignedBlockWithAttestation{Message: &types.BlockWithAttestation{Block: block3}})
	fc.storage.PutState(root3, &types.State{Slot: 3})

	// Add an orphaned fork block at slot 1.
	forkBlock := &types.Block{Slot: 1, ParentRoot: genesisRoot, ProposerIndex: 1, Body: &types.BlockBody{}}
	forkRoot, _ := forkBlock.HashTreeRoot()
	fc.storage.PutBlock(forkRoot, forkBlock)
	fc.storage.PutSignedBlock(forkRoot, &types.SignedBlockWithAttestation{Message: &types.BlockWithAttestation{Block: forkBlock}})
	fc.storage.PutState(forkRoot, &types.State{Slot: 1})

	// Set head to slot 3, finalized at slot 2.
	fc.head = root3
	fc.latestFinalized = &types.Checkpoint{Root: root2, Slot: 2}
	fc.latestJustified = &types.Checkpoint{Root: root2, Slot: 2}

	// Before pruning: 5 blocks (genesis, 1, 2, 3, fork).
	allBlocks := fc.storage.GetAllBlocks()
	if len(allBlocks) != 5 {
		t.Fatalf("expected 5 blocks before pruning, got %d", len(allBlocks))
	}

	fc.pruneStorage(2)

	// After pruning: fork block (slot 1, not canonical) should be removed.
	// Genesis block (slot 0, below finalized, not canonical walk) should be removed.
	// Canonical blocks at slot 1 (below finalized) should be removed.
	// Blocks at slot 2 (== finalized) and slot 3 (> finalized) should remain.
	allBlocks = fc.storage.GetAllBlocks()

	// root2 (finalized) and root3 (head) must remain.
	if _, ok := allBlocks[root2]; !ok {
		t.Fatal("finalized block should not be pruned")
	}
	if _, ok := allBlocks[root3]; !ok {
		t.Fatal("head block should not be pruned")
	}

	// Fork block should be removed.
	if _, ok := allBlocks[forkRoot]; ok {
		t.Fatal("orphan fork block should be pruned")
	}
}

func TestEnforcePayloadCap(t *testing.T) {
	fc := makeTestStore(4)

	// Fill beyond the cap.
	for i := 0; i < maxKnownAggregatedPayloads+100; i++ {
		data := makeAttestationData(uint64(i), uint64(i))
		key, _ := makeAttestationDataKey(data)
		fc.latestKnownAggregatedPayloads[key] = aggregatedPayload{data: data}
	}

	if len(fc.latestKnownAggregatedPayloads) != maxKnownAggregatedPayloads+100 {
		t.Fatalf("expected %d payloads, got %d", maxKnownAggregatedPayloads+100, len(fc.latestKnownAggregatedPayloads))
	}

	fc.enforcePayloadCap()

	if len(fc.latestKnownAggregatedPayloads) > maxKnownAggregatedPayloads {
		t.Fatalf("expected at most %d payloads after cap, got %d", maxKnownAggregatedPayloads, len(fc.latestKnownAggregatedPayloads))
	}
}


func TestPruneOnFinalizationIntegration(t *testing.T) {
	fc := makeTestStore(4)

	// Add stale attestation data, gossip signatures, and aggregated payload cache.
	for i := uint64(1); i <= 3; i++ {
		data := makeAttestationData(i, i)
		key, _ := makeAttestationDataKey(data)
		fc.latestKnownAggregatedPayloads[key] = aggregatedPayload{data: data}

		sigKey := signatureKey{validatorID: 0, dataRoot: [32]byte{byte(i)}}
		fc.gossipSignatures[sigKey] = storedSignature{slot: i, data: data}
		fc.aggregatedPayloads[sigKey] = []storedAggregatedPayload{
			{slot: i, proof: &types.AggregatedSignatureProof{Participants: []byte{0x03}, ProofData: []byte{byte(i)}}},
		}
	}

	// Simulate finalization advancing to slot 2.
	fc.latestFinalized = &types.Checkpoint{Root: fc.head, Slot: 2}

	before := len(fc.latestKnownAggregatedPayloads) + len(fc.gossipSignatures) + len(fc.aggregatedPayloads)
	fc.pruneOnFinalization()
	after := len(fc.latestKnownAggregatedPayloads) + len(fc.gossipSignatures) + len(fc.aggregatedPayloads)

	if after >= before {
		t.Fatalf("pruneOnFinalization should reduce map sizes: before=%d after=%d", before, after)
	}

	// Only slot 3 entries (target > finalized) should remain.
	if len(fc.latestKnownAggregatedPayloads) != 1 {
		t.Fatalf("expected 1 known payload after pruning, got %d", len(fc.latestKnownAggregatedPayloads))
	}
	if len(fc.gossipSignatures) != 1 {
		t.Fatalf("expected 1 gossip signature after pruning, got %d", len(fc.gossipSignatures))
	}
	if len(fc.aggregatedPayloads) != 1 {
		t.Fatalf("expected 1 aggregated payload key after pruning, got %d", len(fc.aggregatedPayloads))
	}
}
