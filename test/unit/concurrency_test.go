package unit

import (
	"sync"
	"testing"

	"github.com/geanlabs/gean/chain/forkchoice"
	"github.com/geanlabs/gean/chain/statetransition"
	"github.com/geanlabs/gean/types"
)

// buildValidBlock builds a valid block for the given slot against the current
// head state in the fork choice store.
func buildValidBlock(t *testing.T, fc *forkchoice.Store, slot uint64) *types.SignedBlockWithAttestation {
	t.Helper()

	status := fc.GetStatus()
	headState, ok := fc.Storage.GetState(status.Head)
	if !ok {
		t.Fatalf("head state not found for slot %d", slot)
	}

	numValidators := uint64(len(headState.Validators))
	proposer := slot % numValidators

	advanced, err := statetransition.ProcessSlots(headState, slot)
	if err != nil {
		t.Fatalf("process_slots(%d): %v", slot, err)
	}
	parentRoot, _ := advanced.LatestBlockHeader.HashTreeRoot()

	block := &types.Block{
		Slot:          slot,
		ProposerIndex: proposer,
		ParentRoot:    parentRoot,
		StateRoot:     types.ZeroHash,
		Body:          &types.BlockBody{Attestations: []*types.Attestation{}},
	}
	postState, err := statetransition.ProcessBlock(advanced, block)
	if err != nil {
		t.Fatalf("process_block(%d): %v", slot, err)
	}
	stateRoot, _ := postState.HashTreeRoot()
	block.StateRoot = stateRoot

	return &types.SignedBlockWithAttestation{
		Message: &types.BlockWithAttestation{Block: block},
	}
}

// TestConcurrentAdvanceTimeAndProcessAttestation verifies that calling
// AdvanceTime and ProcessAttestation concurrently does not race.
func TestConcurrentAdvanceTimeAndProcessAttestation(t *testing.T) {
	fc, _ := makeGenesisFC(5)

	envelope := buildValidBlock(t, fc, 1)
	if err := fc.ProcessBlock(envelope); err != nil {
		t.Fatalf("ProcessBlock: %v", err)
	}

	headRoot := fc.GetStatus().Head

	var wg sync.WaitGroup

	// Goroutines advancing time.
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				fc.AdvanceTime(uint64(1000+id*100+i), i%2 == 0)
			}
		}(g)
	}

	// Goroutines submitting attestations.
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				fc.ProcessAttestation(&types.SignedAttestation{
					Message: &types.Attestation{
						ValidatorID: uint64(id % 5),
						Data: &types.AttestationData{
							Slot:   1,
							Head:   &types.Checkpoint{Root: headRoot, Slot: 1},
							Target: &types.Checkpoint{Root: headRoot, Slot: 1},
							Source: &types.Checkpoint{Root: headRoot, Slot: 0},
						},
					},
				})
			}
		}(g)
	}

	wg.Wait()
}

// TestConcurrentGetStatusWithMutations verifies that GetStatus can be called
// concurrently with AdvanceTime and ProcessAttestation without data races.
func TestConcurrentGetStatusWithMutations(t *testing.T) {
	fc, _ := makeGenesisFC(5)

	envelope := buildValidBlock(t, fc, 1)
	if err := fc.ProcessBlock(envelope); err != nil {
		t.Fatal(err)
	}

	headRoot := fc.GetStatus().Head

	var wg sync.WaitGroup

	// Readers.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				s := fc.GetStatus()
				_ = s.HeadSlot
			}
		}()
	}

	// Writers.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				fc.AdvanceTime(uint64(1000+id*100+j), false)
				fc.ProcessAttestation(&types.SignedAttestation{
					Message: &types.Attestation{
						ValidatorID: uint64(id % 5),
						Data: &types.AttestationData{
							Slot:   1,
							Head:   &types.Checkpoint{Root: headRoot, Slot: 1},
							Target: &types.Checkpoint{Root: headRoot, Slot: 1},
							Source: &types.Checkpoint{Root: headRoot, Slot: 0},
						},
					},
				})
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentProcessBlockAndAdvanceTime verifies that processing blocks
// concurrently with time advances does not race.
func TestConcurrentProcessBlockAndAdvanceTime(t *testing.T) {
	fc, _ := makeGenesisFC(5)

	// Pre-build a valid chain of blocks.
	envelopes := make([]*types.SignedBlockWithAttestation, 5)
	for i := uint64(1); i <= 5; i++ {
		envelopes[i-1] = buildValidBlock(t, fc, i)
		if err := fc.ProcessBlock(envelopes[i-1]); err != nil {
			t.Fatalf("setup ProcessBlock slot %d: %v", i, err)
		}
	}

	// Fresh FC — replay blocks concurrently with time advances.
	fc2, _ := makeGenesisFC(5)

	var wg sync.WaitGroup

	for _, env := range envelopes {
		wg.Add(1)
		go func(e *types.SignedBlockWithAttestation) {
			defer wg.Done()
			_ = fc2.ProcessBlock(e) // errors expected for out-of-order
		}(env)
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				fc2.AdvanceTime(uint64(1000+id*50+j), j%2 == 0)
			}
		}(i)
	}

	wg.Wait()
}
