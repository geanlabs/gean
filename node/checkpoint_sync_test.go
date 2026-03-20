package node

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geanlabs/gean/types"
)

func TestVerifyCheckpointState(t *testing.T) {
	genesisValidators := makeCheckpointValidators(3)
	state := makeCheckpointState(1234, genesisValidators)

	preparedState, stateRoot, blockRoot, err := verifyCheckpointState(state, 1234, genesisValidators)
	if err != nil {
		t.Fatalf("verifyCheckpointState returned error: %v", err)
	}
	if preparedState.LatestBlockHeader.StateRoot != stateRoot {
		t.Fatalf("prepared state root mismatch: got %x want %x", preparedState.LatestBlockHeader.StateRoot, stateRoot)
	}
	if blockRoot == types.ZeroHash {
		t.Fatal("expected non-zero checkpoint block root")
	}
}

func TestVerifyCheckpointStateRejectsValidatorMismatch(t *testing.T) {
	genesisValidators := makeCheckpointValidators(2)
	state := makeCheckpointState(1234, genesisValidators)
	state.Validators[1].Pubkey[0] = 0xFF

	_, _, _, err := verifyCheckpointState(state, 1234, genesisValidators)
	if err == nil {
		t.Fatal("expected validator mismatch error")
	}
}

func TestVerifyCheckpointStateRejectsMissingCanonicalHistory(t *testing.T) {
	genesisValidators := makeCheckpointValidators(2)
	state := makeCheckpointState(1234, genesisValidators)
	state.HistoricalBlockHashes = nil

	_, _, _, err := verifyCheckpointState(state, 1234, genesisValidators)
	if err == nil {
		t.Fatal("expected missing canonical history error")
	}
}

func TestDownloadCheckpointState(t *testing.T) {
	state := makeCheckpointState(1234, makeCheckpointValidators(2))
	payload, err := state.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ returned error: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	downloadedState, err := downloadCheckpointState(server.URL)
	if err != nil {
		t.Fatalf("downloadCheckpointState returned error: %v", err)
	}
	if downloadedState.Config.GenesisTime != 1234 {
		t.Fatalf("downloaded genesis time = %d, want 1234", downloadedState.Config.GenesisTime)
	}
}

func makeCheckpointState(genesisTime uint64, validators []*types.Validator) *types.State {
	emptyBody := &types.BlockBody{Attestations: []*types.AggregatedAttestation{}}
	bodyRoot, _ := emptyBody.HashTreeRoot()
	stateValidators := make([]*types.Validator, len(validators))
	for i, validator := range validators {
		copyValidator := *validator
		stateValidators[i] = &copyValidator
	}

	return &types.State{
		Config: &types.Config{GenesisTime: genesisTime},
		Slot:   3,
		LatestBlockHeader: &types.BlockHeader{
			Slot:          3,
			ProposerIndex: 0,
			ParentRoot:    [32]byte{0x11},
			StateRoot:     types.ZeroHash,
			BodyRoot:      bodyRoot,
		},
		LatestJustified:          &types.Checkpoint{Root: [32]byte{0x11}, Slot: 2},
		LatestFinalized:          &types.Checkpoint{Root: [32]byte{0x22}, Slot: 1},
		HistoricalBlockHashes:    [][32]byte{{0x33}, {0x22}, {0x11}},
		JustifiedSlots:           []byte{0x01},
		Validators:               stateValidators,
		JustificationsRoots:      [][32]byte{},
		JustificationsValidators: []byte{0x01},
	}
}

func makeCheckpointValidators(n int) []*types.Validator {
	validators := make([]*types.Validator, n)
	for i := range validators {
		validators[i] = &types.Validator{
			Index:  uint64(i),
			Pubkey: [52]byte{byte(i + 1)},
		}
	}
	return validators
}
