package node

import (
	"sync"
	"testing"

	"github.com/geanlabs/gean/storage"
	"github.com/geanlabs/gean/types"
	"github.com/geanlabs/gean/xmss"
)

// One-time benchmark fixture: a real XMSS keypair + one pre-aggregated
// signature proof + a consensus store whose target-state resolves to a valid
// validator. Each bench builds a block by cloning N copies of the same
// (attestation, proof) pair and signing the resulting block with the fixture
// keypair. The verify path doesn't care that the attestations are
// semantically identical — it runs N FFI calls regardless, which is exactly
// the cost we want to measure.
var (
	benchFixtureOnce sync.Once
	benchFixtureErr  error
	benchKeyPair     *xmss.ValidatorKeyPair
	benchState       *types.State
	benchStore       *ConsensusStore
	benchTargetRoot  [32]byte
	benchAtt         *types.AggregatedAttestation
	benchProof       *types.AggregatedSignatureProof
)

func loadBenchFixture(tb testing.TB) {
	tb.Helper()
	benchFixtureOnce.Do(buildBenchFixture)
	if benchFixtureErr != nil {
		tb.Fatalf("benchmark fixture: %v", benchFixtureErr)
	}
}

func buildBenchFixture() {
	xmss.EnsureProverReady()
	xmss.EnsureVerifierReady()

	kp, err := xmss.GenerateKeyPair("bench-validator", 0, 1<<18)
	if err != nil {
		benchFixtureErr = err
		return
	}
	benchKeyPair = kp // deliberately NOT closed — fixture lives for the process

	pkBytes, err := kp.PublicKeyBytes()
	if err != nil {
		benchFixtureErr = err
		return
	}

	// One validator whose attestation + proposal pubkey are the same key.
	// verifyBlockSignatures reads them as separate fields but doesn't care
	// that they happen to be equal here; it only needs the signatures to
	// verify against the pubkey it looks up from the state.
	benchState = &types.State{
		Config:            &types.ChainConfig{GenesisTime: 1000},
		Slot:              0,
		LatestBlockHeader: &types.BlockHeader{},
		LatestJustified:   &types.Checkpoint{Slot: 0},
		LatestFinalized:   &types.Checkpoint{Slot: 0},
		Validators: []*types.Validator{{
			AttestationPubkey: pkBytes,
			ProposalPubkey:    pkBytes,
			Index:             0,
		}},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}

	// Use the state's own hash as the target root so GetState(target.Root)
	// in verifyBlockSignatures returns benchState.
	stateRoot, err := benchState.HashTreeRoot()
	if err != nil {
		benchFixtureErr = err
		return
	}
	benchTargetRoot = stateRoot

	attData := &types.AttestationData{
		Slot:   1,
		Head:   &types.Checkpoint{Root: benchTargetRoot, Slot: 1},
		Target: &types.Checkpoint{Root: benchTargetRoot, Slot: 1},
		Source: &types.Checkpoint{Root: benchTargetRoot, Slot: 0},
	}
	dataRoot, err := attData.HashTreeRoot()
	if err != nil {
		benchFixtureErr = err
		return
	}

	sigBytes, err := kp.Sign(uint32(attData.Slot), dataRoot)
	if err != nil {
		benchFixtureErr = err
		return
	}

	cpk, err := xmss.ParsePublicKey(pkBytes)
	if err != nil {
		benchFixtureErr = err
		return
	}
	// cpk lifetime is owned by the test — not freed, matches PubKeyCache model.

	csig, err := xmss.ParseSignature(sigBytes[:])
	if err != nil {
		benchFixtureErr = err
		return
	}
	defer xmss.FreeSignature(csig)

	proofBytes, err := xmss.AggregateSignatures(
		[]xmss.CPubKey{cpk},
		[]xmss.CSig{csig},
		dataRoot,
		uint32(attData.Slot),
	)
	if err != nil {
		benchFixtureErr = err
		return
	}

	aggBits := types.NewBitlistSSZ(1)
	types.BitlistSet(aggBits, 0)

	benchAtt = &types.AggregatedAttestation{
		AggregationBits: aggBits,
		Data:            attData,
	}
	benchProof = &types.AggregatedSignatureProof{
		Participants: aggBits,
		ProofData:    proofBytes,
	}

	benchStore = NewConsensusStore(storage.NewInMemoryBackend())
	benchStore.InsertState(benchTargetRoot, benchState)
}

// buildBenchSignedBlock builds a SignedBlock with n copies of the fixture's
// (attestation, proof) pair and a fresh proposer signature over the resulting
// block root. Called once per bench run, outside the timed loop.
//
// block.Slot is keyed off n so each bench (_1, _8, _16) signs at a distinct
// XMSS slot. Reusing a single slot across different block-root messages with
// the same key would be a WOTS+ hygiene break even though the verifier won't
// catch it; the bench file models correct usage.
func buildBenchSignedBlock(n int) (*types.SignedBlock, error) {
	atts := make([]*types.AggregatedAttestation, n)
	proofs := make([]*types.AggregatedSignatureProof, n)
	for i := 0; i < n; i++ {
		atts[i] = benchAtt
		proofs[i] = benchProof
	}

	block := &types.Block{
		Slot:          uint64(2 + n),
		ProposerIndex: 0,
		Body:          &types.BlockBody{Attestations: atts},
	}
	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		return nil, err
	}

	proposerSig, err := benchKeyPair.Sign(uint32(block.Slot), blockRoot)
	if err != nil {
		return nil, err
	}

	return &types.SignedBlock{
		Block: block,
		Signature: &types.BlockSignatures{
			ProposerSignature:     proposerSig,
			AttestationSignatures: proofs,
		},
	}, nil
}

func benchN(b *testing.B, n int) {
	if testing.Short() {
		b.Skip("skipping XMSS benchmark in -short mode")
	}
	loadBenchFixture(b)

	signedBlock, err := buildBenchSignedBlock(n)
	if err != nil {
		b.Fatalf("build signed block: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := verifyBlockSignatures(benchStore, signedBlock, benchState); err != nil {
			b.Fatalf("verify failed: %v", err)
		}
	}
}

func BenchmarkVerifyBlockSignatures_1(b *testing.B)  { benchN(b, 1) }
func BenchmarkVerifyBlockSignatures_8(b *testing.B)  { benchN(b, 8) }
func BenchmarkVerifyBlockSignatures_16(b *testing.B) { benchN(b, 16) }
