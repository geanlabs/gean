package blockprocessor

import (
	"sync"
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

var (
	benchFixtureOnce sync.Once
	benchFixtureErr  error
	benchKeyPair     *xmss.ValidatorKeyPair
	benchState       *types.State
	benchStore       *store.ConsensusStore
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
	benchKeyPair = kp

	pkBytes, err := kp.PublicKeyBytes()
	if err != nil {
		benchFixtureErr = err
		return
	}

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

	attSlot, err := slot32(attData.Slot)
	if err != nil {
		benchFixtureErr = err
		return
	}
	sigBytes, err := kp.Sign(attSlot, dataRoot)
	if err != nil {
		benchFixtureErr = err
		return
	}

	cpk, err := xmss.ParsePublicKey(pkBytes)
	if err != nil {
		benchFixtureErr = err
		return
	}
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
		attSlot,
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

	benchStore = store.NewConsensusStore(storage.NewInMemoryBackend())
	benchStore.InsertState(benchTargetRoot, benchState)
}

func buildBenchSignedBlock(n int) (*types.SignedBlock, error) {
	atts := make([]*types.AggregatedAttestation, n)
	proofs := make([]*types.AggregatedSignatureProof, n)
	for i := range n {
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

	blockSlot, err := slot32(block.Slot)
	if err != nil {
		return nil, err
	}
	proposerSig, err := benchKeyPair.Sign(blockSlot, blockRoot)
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
