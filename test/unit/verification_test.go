package unit

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/xmss/leansig"
	"github.com/geanlabs/gean/storage/memory"
	"github.com/geanlabs/gean/types"
)

// TestVerification checks that ProcessBlock and ProcessAttestation
// correctly verify signatures.
func TestVerification(t *testing.T) {
	// 1. Setup genesis state with 1 validator.
	seed := uint64(time.Now().UnixNano())
	kp, err := leansig.GenerateKeypair(seed, 0, 8)
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}
	defer kp.Free()

	pubkey, err := kp.PublicKeyBytes()
	if err != nil {
		t.Fatalf("failed to get pubkey: %v", err)
	}
	t.Logf("Validation: Public key size = %d bytes", len(pubkey))

	var pk32 [52]byte
	copy(pk32[:], pubkey)

	validator := &types.Validator{
		Pubkey: pk32,
		Index:  0,
	}
	state := statetransition.GenerateGenesis(1000, []*types.Validator{validator})
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

	// 2. Test ProcessBlock Verification

	// Use ProduceBlock to create a valid signed block.
	// Note: ProduceBlock puts the block in storage!
	// So ProcessBlock(envelope) will return "already known" (nil) without verifying signatures.
	// To test verification, we must use a block NOT in storage.
	// We can generate envelope2 (slot 2), which is in storage.
	// But if we corrupt it, the HASH matches the one in storage (if we don't change body).
	// ProduceBlock puts block by hash in storage.
	// So we need to generate envelope, then DELETE it from storage?
	// or Use ProduceBlock on a DIFFERENT store, then feed to 'fc'.

	// Better approach: Use separate FC instance for generating blocks.
	genStore := memory.New()
	genFc := forkchoice.NewStore(state, genesisBlock, genStore)

	// Generate valid blocks using genFc
	envelope1, err := genFc.ProduceBlock(1, 0, kp)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Validation: Signature size (envelope) = %d bytes", len(envelope1.Signature[0]))

	// Check raw signature size
	msgRoot1, _ := envelope1.Message.HashTreeRoot()
	rawSig, err := kp.Sign(0, msgRoot1) // epoch 0
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Validation: Raw Signature size = %d bytes", len(rawSig))

	// Process valid block 1 in main fc
	if err := fc.ProcessBlock(envelope1); err != nil {
		t.Fatalf("ProcessBlock failed with valid signature: %v", err)
	}

	// Generate block 2
	envelope2, err := genFc.ProduceBlock(2, 0, kp)
	if err != nil {
		t.Fatal(err)
	}

	// 2b. Verify invalid block signature
	// We must ensure ProcessBlock runs verification. Since envelope2 is NOT in 'fc' storage, it will run.

	// Corrupt signature (deep copy first to preserve valid one)
	envInvalid := deepCopyBlockEnvelope(envelope2)
	lastIdx := len(envInvalid.Signature) - 1
	rand.Read(envInvalid.Signature[lastIdx][:])

	// verifyAttestationSignature check inside ProcessBlock
	if err := fc.ProcessBlock(envInvalid); err == nil {
		t.Error("ProcessBlock accepted invalid signature")
	} else {
		t.Logf("ProcessBlock correctly rejected invalid signature: %v", err)
	}

	// 2c. Verify valid block 2
	if err := fc.ProcessBlock(envelope2); err != nil {
		t.Errorf("ProcessBlock failed with valid signature (block 2): %v", err)
	}

	// 3. Test ProcessAttestation Verification
	// Produce attestation from genFc (so time advances there)
	// But 'fc' time is still Genesis (ProcessBlock doesn't advance time).
	// We must advance 'fc' time to accept future attestation.
	fc.Time = 10 * types.IntervalsPerSlot // current slot 10, well ahead of attestation slot 2

	sa, err := fc.ProduceAttestation(2, 0, kp) // Attest to block 2
	if err != nil {
		t.Fatalf("ProduceAttestation failed: %v", err)
	}

	// Explicitly check signature validity
	dataRoot, _ := sa.Message.HashTreeRoot()
	ep := uint32(sa.Message.Target.Slot / types.SlotsPerEpoch)
	if err := leansig.Verify(pubkey[:], ep, dataRoot, sa.Signature[:]); err != nil {
		t.Fatalf("Generated attestation has invalid signature: %v", err)
	}

	// 3a. Verify valid attestation processing
	fc.ProcessAttestation(sa)

	// Note: fc.mu is unexported, so we cannot lock/unlock.
	// We assume ProcessAttestation is synchronous and efficient enough that
	// checking LatestNewAttestations immediately after is safe in this single-threaded test.
	if _, ok := fc.LatestNewAttestations[0]; !ok {
		t.Error("Valid attestation not added to store")
	}

	// 3b. Verify invalid attestation signature
	saInvalid := deepCopySignedAttestation(sa)
	rand.Read(saInvalid.Signature[:])

	// Reset store
	// Cannot lock mu. Delete map entry directly.
	// Map access is unsafe if concurrent, but we are sequential here.
	delete(fc.LatestNewAttestations, 0)

	fc.ProcessAttestation(saInvalid)

	if _, ok := fc.LatestNewAttestations[0]; ok {
		t.Error("Invalid attestation ADDED to store")
	}
}

func deepCopyBlockEnvelope(src *types.SignedBlockWithAttestation) *types.SignedBlockWithAttestation {
	// Minimal deep copy for signature corruption
	dst := *src
	dst.Signature = make([][3112]byte, len(src.Signature))
	copy(dst.Signature, src.Signature)
	return &dst
}

func deepCopySignedAttestation(src *types.SignedAttestation) *types.SignedAttestation {
	dst := *src
	// Signature is array, copied by value in *dst.
	// Message is pointer, but we don't modify message.
	return &dst
}
