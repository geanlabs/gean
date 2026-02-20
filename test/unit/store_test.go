package unit

import (
	"testing"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/storage/memory"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss/leansig"
)

func makeGenesisFC(numValidators uint64) (*forkchoice.Store, *types.State) {
	state := statetransition.GenerateGenesis(1000, makeTestValidators(numValidators))

	emptyBody := &types.BlockBody{Attestations: []*types.Attestation{}}
	genesisBlock := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash,
		Body:          emptyBody,
	}
	stateRoot, _ := state.HashTreeRoot()
	genesisBlock.StateRoot = stateRoot

	store := memory.New()
	fc := forkchoice.NewStore(state, genesisBlock, store)
	return fc, state
}

func TestForkChoiceInitFromGenesis(t *testing.T) {
	fc, _ := makeGenesisFC(5)

	if fc.Head == types.ZeroHash {
		t.Error("head should not be zero after genesis")
	}
	if fc.LatestJustified.Root == types.ZeroHash {
		t.Error("justified root should be anchor root, not zero")
	}
	if fc.LatestJustified.Root != fc.Head {
		t.Errorf("justified root (%x) != head (%x)", fc.LatestJustified.Root[:4], fc.Head[:4])
	}
	if fc.LatestFinalized.Root != fc.Head {
		t.Errorf("finalized root (%x) != head (%x)", fc.LatestFinalized.Root[:4], fc.Head[:4])
	}
}

func TestAnchorRootSourcePassesValidation(t *testing.T) {
	// Use a real keypair so signature verification passes.
	kp, err := leansig.GenerateKeypair(42, 0, 8)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	defer kp.Free()

	pubkey, err := kp.PublicKeyBytes()
	if err != nil {
		t.Fatalf("pubkey: %v", err)
	}
	var pk [52]byte
	copy(pk[:], pubkey)

	// Use buildForkChoiceWithBlocks so we have at least 1 block.
	fc, _ := buildForkChoiceWithBlocks(t, 5, 2)

	// Patch validator 1's pubkey in all stored states.
	for _, st := range fc.Storage.GetAllStates() {
		st.Validators[1].Pubkey = pk
	}

	// The key assertion: source root should be anchor root (not ZeroHash).
	if fc.LatestJustified.Root == types.ZeroHash {
		t.Fatal("justified root is zero — store init bug not fixed")
	}

	fc.Time = 10 * types.IntervalsPerSlot

	sa, err := fc.ProduceAttestation(2, 1, kp)
	if err != nil {
		t.Fatalf("ProduceAttestation: %v", err)
	}

	// Source should use the non-zero justified root.
	if sa.Message.Data.Source.Root == types.ZeroHash {
		t.Fatal("attestation source root is zero — should be anchor root")
	}

	fc.ProcessAttestation(sa)

	if _, ok := fc.LatestNewAttestations[1]; !ok {
		t.Fatal("attestation with anchor root source should pass validation")
	}
}

func TestForkChoiceTickInterval(t *testing.T) {
	fc, _ := makeGenesisFC(5)
	initialTime := fc.Time

	fc.TickInterval(false)
	if fc.Time != initialTime+1 {
		t.Errorf("time = %d, want %d", fc.Time, initialTime+1)
	}
}

func TestForkChoiceAcceptNewAttestations(t *testing.T) {
	fc, _ := makeGenesisFC(5)

	// Add an attestation to new attestations.
	fc.LatestNewAttestations[0] = &types.SignedAttestation{
		Message: &types.Attestation{
			ValidatorID: 0,
			Data: &types.AttestationData{
				Slot:   0,
				Head:   &types.Checkpoint{Root: fc.Head, Slot: 0},
				Target: &types.Checkpoint{Root: fc.Head, Slot: 0},
				Source: &types.Checkpoint{Root: fc.Head, Slot: 0},
			},
		},
	}

	fc.AcceptNewAttestations()

	if len(fc.LatestNewAttestations) != 0 {
		t.Error("new attestations should be empty after accept")
	}
	if _, ok := fc.LatestKnownAttestations[0]; !ok {
		t.Error("attestation should be in known attestations after accept")
	}
}

func TestForkChoiceInitPanicsOnAnchorStateRootMismatch(t *testing.T) {
	state := statetransition.GenerateGenesis(1000, makeTestValidators(5))
	emptyBody := &types.BlockBody{Attestations: []*types.Attestation{}}
	genesisBlock := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash, // intentionally wrong
		Body:          emptyBody,
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for anchor block/state root mismatch")
		}
	}()
	_ = forkchoice.NewStore(state, genesisBlock, memory.New())
}

func TestProduceAttestationAcceptsNewAttestationsFirst(t *testing.T) {
	fc, _ := makeGenesisFC(5)
	fc.LatestNewAttestations[3] = &types.SignedAttestation{
		Message: &types.Attestation{
			ValidatorID: 3,
			Data: &types.AttestationData{
				Slot:   0,
				Head:   &types.Checkpoint{Root: fc.Head, Slot: 0},
				Target: &types.Checkpoint{Root: fc.Head, Slot: 0},
				Source: &types.Checkpoint{Root: fc.Head, Slot: 0},
			},
		},
	}

	_, _ = fc.ProduceAttestation(1, 0, newTestSigner())

	if len(fc.LatestNewAttestations) != 0 {
		t.Fatalf("expected latest_new_attestations to be drained, got %d entries", len(fc.LatestNewAttestations))
	}
	if _, ok := fc.LatestKnownAttestations[3]; !ok {
		t.Fatal("expected attestation to be moved into latest_known_attestations")
	}
}

// TestJustifiedMonotonicity verifies that processing a block on a fork with
// a lower justified checkpoint does not regress the store's LatestJustified.
func TestJustifiedMonotonicity(t *testing.T) {
	fc, _ := makeGenesisFC(5)
	initialJustified := fc.LatestJustified.Slot

	// Build a chain of blocks. Justified should never decrease.
	state := getGenesisState(5)
	parentRoot := fc.Head

	for slot := uint64(1); slot <= 5; slot++ {
		advanced, err := statetransition.ProcessSlots(state, slot)
		if err != nil {
			t.Fatalf("process slots(%d): %v", slot, err)
		}
		headerRoot, err := advanced.LatestBlockHeader.HashTreeRoot()
		if err != nil {
			t.Fatalf("header root(%d): %v", slot, err)
		}
		block := &types.Block{
			Slot:          slot,
			ProposerIndex: slot % 5,
			ParentRoot:    headerRoot,
			StateRoot:     types.ZeroHash,
			Body:          &types.BlockBody{Attestations: []*types.Attestation{}},
		}
		postState, err := statetransition.ProcessBlock(advanced, block)
		if err != nil {
			t.Fatalf("process block(%d): %v", slot, err)
		}
		sr, _ := postState.HashTreeRoot()
		block.StateRoot = sr

		state, err = statetransition.StateTransition(state, block)
		if err != nil {
			t.Fatalf("state transition(%d): %v", slot, err)
		}

		envelope := &types.SignedBlockWithAttestation{
			Message: &types.BlockWithAttestation{Block: block},
		}
		if err := fc.ProcessBlock(envelope); err != nil {
			t.Fatalf("forkchoice process block(%d): %v", slot, err)
		}

		bh, _ := block.HashTreeRoot()
		parentRoot = bh

		// Justified slot must never decrease.
		if fc.LatestJustified.Slot < initialJustified {
			t.Fatalf("justified slot regressed at slot %d: %d < %d",
				slot, fc.LatestJustified.Slot, initialJustified)
		}
	}
	_ = parentRoot
}

// TestFinalizedMonotonicity verifies that LatestFinalized never regresses,
// even when the head changes.
func TestFinalizedMonotonicity(t *testing.T) {
	fc, _ := makeGenesisFC(5)
	initialFinalized := fc.LatestFinalized.Slot

	state := getGenesisState(5)
	for slot := uint64(1); slot <= 5; slot++ {
		advanced, err := statetransition.ProcessSlots(state, slot)
		if err != nil {
			t.Fatalf("process slots(%d): %v", slot, err)
		}
		headerRoot, err := advanced.LatestBlockHeader.HashTreeRoot()
		if err != nil {
			t.Fatalf("header root(%d): %v", slot, err)
		}
		block := &types.Block{
			Slot:          slot,
			ProposerIndex: slot % 5,
			ParentRoot:    headerRoot,
			StateRoot:     types.ZeroHash,
			Body:          &types.BlockBody{Attestations: []*types.Attestation{}},
		}
		postState, err := statetransition.ProcessBlock(advanced, block)
		if err != nil {
			t.Fatalf("process block(%d): %v", slot, err)
		}
		sr, _ := postState.HashTreeRoot()
		block.StateRoot = sr

		state, err = statetransition.StateTransition(state, block)
		if err != nil {
			t.Fatalf("state transition(%d): %v", slot, err)
		}

		envelope := &types.SignedBlockWithAttestation{
			Message: &types.BlockWithAttestation{Block: block},
		}
		if err := fc.ProcessBlock(envelope); err != nil {
			t.Fatalf("forkchoice process block(%d): %v", slot, err)
		}

		// Finalized slot must never decrease.
		if fc.LatestFinalized.Slot < initialFinalized {
			t.Fatalf("finalized slot regressed at slot %d: %d < %d",
				slot, fc.LatestFinalized.Slot, initialFinalized)
		}
	}
}

// TestAttestationSupersedingUsesSlot verifies that attestation superseding
// uses data.Slot (attestation slot), not data.Target.Slot. This exercises
// the full production ProcessAttestation path including validation and
// signature verification.
func TestAttestationSupersedingUsesSlot(t *testing.T) {
	// Generate a real XMSS keypair so signature verification passes.
	kp, err := leansig.GenerateKeypair(42, 0, 8)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	defer kp.Free()

	pubkey, err := kp.PublicKeyBytes()
	if err != nil {
		t.Fatalf("pubkey: %v", err)
	}
	var pk [52]byte
	copy(pk[:], pubkey)

	// Build chain with blocks at slots 0-3.
	fc, _ := buildForkChoiceWithBlocks(t, 5, 3)

	// Patch validator 0's pubkey in all stored states.
	for _, st := range fc.Storage.GetAllStates() {
		st.Validators[0].Pubkey = pk
	}

	// Set time far ahead so both attestations pass gossip validation.
	fc.Time = 10 * types.IntervalsPerSlot

	// Produce two signed attestations before either is processed.
	// attA at slot 3 (higher), attB at slot 2 (lower).
	attA, err := fc.ProduceAttestation(3, 0, kp)
	if err != nil {
		t.Fatalf("ProduceAttestation(slot=3): %v", err)
	}
	attB, err := fc.ProduceAttestation(2, 0, kp)
	if err != nil {
		t.Fatalf("ProduceAttestation(slot=2): %v", err)
	}

	// Process attA (slot=3) first, then attB (slot=2).
	// attB should NOT supersede attA because attB.Slot < attA.Slot.
	fc.ProcessAttestation(attA)
	fc.ProcessAttestation(attB)

	got, ok := fc.LatestNewAttestations[0]
	if !ok {
		t.Fatal("expected validator 0 attestation in LatestNewAttestations")
	}
	if got.Message.Data.Slot != 3 {
		t.Fatalf("expected attestation with slot=3 to remain, got slot=%d", got.Message.Data.Slot)
	}
}

// getGenesisState is a helper that returns a fresh genesis state.
func getGenesisState(numValidators uint64) *types.State {
	return statetransition.GenerateGenesis(1000, makeTestValidators(numValidators))
}

// TestReorgOnNewlyJustifiedCheckpoint verifies that when a competing fork
// includes body attestations that justify a new checkpoint, the store's head
// reorganizes to the fork containing the justified block, and the justified
// checkpoint never regresses.
func TestReorgOnNewlyJustifiedCheckpoint(t *testing.T) {
	// Generate a real XMSS keypair so signature verification succeeds.
	kp, err := leansig.GenerateKeypair(42, 0, 8)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	defer kp.Free()

	pubkey, err := kp.PublicKeyBytes()
	if err != nil {
		t.Fatalf("pubkey: %v", err)
	}
	var pk [52]byte
	copy(pk[:], pubkey)

	// Create genesis with real pubkeys so body attestation signatures verify.
	numValidators := uint64(5)
	validators := make([]*types.Validator, numValidators)
	for i := range validators {
		validators[i] = &types.Validator{Pubkey: pk, Index: uint64(i)}
	}
	genesisState := statetransition.GenerateGenesis(1000, validators)
	emptyBody := &types.BlockBody{Attestations: []*types.Attestation{}}
	genesisBlock := &types.Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    types.ZeroHash,
		StateRoot:     types.ZeroHash,
		Body:          emptyBody,
	}
	sr, _ := genesisState.HashTreeRoot()
	genesisBlock.StateRoot = sr

	store := memory.New()
	fc := forkchoice.NewStore(genesisState, genesisBlock, store)

	// --- Fork A: genesis → slot 1 (head = blockA1) ---
	advA1, err := statetransition.ProcessSlots(genesisState, 1)
	if err != nil {
		t.Fatalf("processSlots A1: %v", err)
	}
	parentRootA1, _ := advA1.LatestBlockHeader.HashTreeRoot()
	blockA1 := &types.Block{
		Slot:          1,
		ProposerIndex: 1,
		ParentRoot:    parentRootA1,
		StateRoot:     types.ZeroHash,
		Body:          emptyBody,
	}
	postA1, err := statetransition.ProcessBlock(advA1, blockA1)
	if err != nil {
		t.Fatalf("processBlock A1: %v", err)
	}
	srA1, _ := postA1.HashTreeRoot()
	blockA1.StateRoot = srA1

	envA1 := &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{Block: blockA1},
	}
	if err := fc.ProcessBlock(envA1); err != nil {
		t.Fatalf("fc processBlock A1: %v", err)
	}

	forkAHead := fc.Head
	initialJustifiedSlot := fc.LatestJustified.Slot

	// Advance time so attestation validation passes (currentSlot = 10).
	fc.Time = 10 * types.IntervalsPerSlot

	// --- Fork B: genesis → slot 2B (empty body, creates fork) ---
	advB2, err := statetransition.ProcessSlots(genesisState, 2)
	if err != nil {
		t.Fatalf("processSlots B2: %v", err)
	}
	parentRootB2, _ := advB2.LatestBlockHeader.HashTreeRoot()
	blockB2 := &types.Block{
		Slot:          2,
		ProposerIndex: 2,
		ParentRoot:    parentRootB2,
		StateRoot:     types.ZeroHash,
		Body:          emptyBody,
	}
	postB2, err := statetransition.ProcessBlock(advB2, blockB2)
	if err != nil {
		t.Fatalf("processBlock B2: %v", err)
	}
	srB2, _ := postB2.HashTreeRoot()
	blockB2.StateRoot = srB2

	envB2 := &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{Block: blockB2},
	}
	if err := fc.ProcessBlock(envB2); err != nil {
		t.Fatalf("fc processBlock B2: %v", err)
	}
	blockB2Hash, _ := blockB2.HashTreeRoot()

	// --- Fork B: slot 2B → slot 4B (body attestations justify slot 2) ---
	stateB2, ok := fc.Storage.GetState(blockB2Hash)
	if !ok {
		t.Fatal("stateB2 not found in store")
	}
	advB4, err := statetransition.ProcessSlots(stateB2, 4)
	if err != nil {
		t.Fatalf("processSlots B4: %v", err)
	}
	parentRootB4, _ := advB4.LatestBlockHeader.HashTreeRoot()

	// After ProcessBlockHeader for slot 4B, HistoricalBlockHashes will be:
	//   [0]: genesis header hash, [1]: ZeroHash (empty slot 1),
	//   [2]: slot 2B header hash (= parentRootB4 = blockB2Hash), [3]: ZeroHash
	genesisHeaderHash := stateB2.HistoricalBlockHashes[0]
	slot2BHeaderHash := parentRootB4

	// Sanity: block hash equals header hash after StateRoot fill.
	if blockB2Hash != slot2BHeaderHash {
		t.Fatalf("blockB2Hash (%x) != slot2BHeaderHash (%x)",
			blockB2Hash[:4], slot2BHeaderHash[:4])
	}

	// 4 body attestations: source = justified genesis, target = slot 2B.
	// Supermajority (4/5 ≥ 2/3) justifies slot 2 on fork B.
	bodyAtts := make([]*types.Attestation, 4)
	for i := uint64(0); i < 4; i++ {
		bodyAtts[i] = &types.Attestation{
			ValidatorID: i,
			Data: &types.AttestationData{
				Slot:   2,
				Head:   &types.Checkpoint{Root: blockB2Hash, Slot: 2},
				Source: &types.Checkpoint{Root: genesisHeaderHash, Slot: 0},
				Target: &types.Checkpoint{Root: slot2BHeaderHash, Slot: 2},
			},
		}
	}

	blockB4 := &types.Block{
		Slot:          4,
		ProposerIndex: 4,
		ParentRoot:    parentRootB4,
		StateRoot:     types.ZeroHash,
		Body:          &types.BlockBody{Attestations: bodyAtts},
	}
	postB4, err := statetransition.ProcessBlock(advB4, blockB4)
	if err != nil {
		t.Fatalf("processBlock B4: %v", err)
	}
	srB4, _ := postB4.HashTreeRoot()
	blockB4.StateRoot = srB4

	// Sign each body attestation with the real keypair.
	sigs := make([][types.XMSSSignatureSize]byte, 4)
	for i := 0; i < 4; i++ {
		attRoot, _ := bodyAtts[i].HashTreeRoot()
		signingSlot := uint32(bodyAtts[i].Data.Slot)
		sigBytes, err := kp.Sign(signingSlot, attRoot)
		if err != nil {
			t.Fatalf("sign attestation %d: %v", i, err)
		}
		copy(sigs[i][:], sigBytes)
	}

	envB4 := &types.SignedBlockWithAttestation{
		Message:   &types.BlockWithAttestation{Block: blockB4},
		Signature: sigs,
	}
	if err := fc.ProcessBlock(envB4); err != nil {
		t.Fatalf("fc processBlock B4: %v", err)
	}

	blockB4Hash, _ := blockB4.HashTreeRoot()

	// Head should have reorged from fork A (slot 1) to fork B (slot 4).
	if fc.Head == forkAHead {
		t.Fatal("expected head to reorg from fork A to fork B")
	}
	if fc.Head != blockB4Hash {
		t.Errorf("expected head = blockB4 (%x), got %x",
			blockB4Hash[:4], fc.Head[:4])
	}

	// Justified checkpoint must never regress (monotonicity).
	if fc.LatestJustified.Slot < initialJustifiedSlot {
		t.Fatalf("justified slot regressed: %d < %d",
			fc.LatestJustified.Slot, initialJustifiedSlot)
	}

	// Justification should have advanced to slot 2 via body attestations.
	if fc.LatestJustified.Slot != 2 {
		t.Errorf("expected justified slot = 2, got %d", fc.LatestJustified.Slot)
	}
}
