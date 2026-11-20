//go:build spectests

package lean

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/geanlabs/gean/internal/core/logger"
	"github.com/geanlabs/gean/internal/core/storage"
	"github.com/geanlabs/gean/internal/lean/importer"
	"github.com/geanlabs/gean/internal/lean/specfixtures"
	"github.com/geanlabs/gean/internal/lean/store"
	"github.com/geanlabs/gean/internal/lean/types"
)

// Test runner.

func TestSpecSignatures(t *testing.T) {
	logger.SetOutput(io.Discard)
	defer logger.SetOutput(os.Stderr)

	fixtureDir := "../../../leanSpec/fixtures/consensus/verify_signatures"

	if _, err := os.Stat(fixtureDir); os.IsNotExist(err) {
		t.Skipf("fixtures not present at %s; skipping", fixtureDir)
	}

	var files []string
	err := filepath.Walk(fixtureDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".json" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking fixture dir %s: %v", fixtureDir, err)
	}

	if len(files) == 0 {
		t.Skipf("no fixture files found in %s; skipping", fixtureDir)
	}

	for _, file := range files {
		file := file
		relPath, _ := filepath.Rel(fixtureDir, file)
		t.Run(relPath, func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("reading %s: %v", file, err)
			}

			var fixture map[string]specfixtures.VerifySignaturesFixture
			if err := json.Unmarshal(data, &fixture); err != nil {
				t.Fatalf("unmarshalling %s: %v", file, err)
			}

			for testName, tt := range fixture {
				tt := tt
				t.Run(testName, func(t *testing.T) {
					runSignatureTest(t, &tt)
				})
			}
		})
	}
}

func runSignatureTest(t *testing.T, tt *specfixtures.VerifySignaturesFixture) {
	t.Helper()

	anchorState, err := tt.AnchorState.ToState()
	if err != nil {
		t.Fatalf("anchor state: %v", err)
	}

	// A genesis header has a zero state root; fill it before computing the anchor
	// block root, as the node does at startup (internal/lean/store.go).
	stateRoot, _ := anchorState.HashTreeRoot()
	header := anchorState.LatestBlockHeader
	if header.StateRoot == types.ZeroRoot {
		header.StateRoot = stateRoot
	}
	anchorRoot, err := header.HashTreeRoot()
	if err != nil {
		t.Fatalf("computing anchor block root: %v", err)
	}

	backend := storage.NewInMemoryBackend()
	s := store.NewConsensusStore(backend)

	if err := s.PutAnchor(anchorRoot, header, anchorState); err != nil {
		t.Fatalf("put anchor: %v", err)
	}

	signedBlock, err := tt.SignedBlock.ToSignedBlock()
	if err != nil {
		t.Fatalf("signed block: %v", err)
	}

	// Place the store clock at the block's slot, as the live node does before
	// importing an on-time block; otherwise the future-horizon guard would reject a
	// block whose slot leads the default-zero clock.
	if signedBlock != nil && signedBlock.Block != nil {
		s.SetTime(signedBlock.Block.Slot * types.IntervalsPerSlot)
	}

	err = importer.OnBlock(s, signedBlock)

	expectFailure := tt.RejectionReason != nil

	if expectFailure {
		if err == nil {
			t.Fatalf("expected failure (%q) but OnBlock succeeded", *tt.RejectionReason)
		}
	} else {
		if err != nil {
			t.Fatalf("expected success but OnBlock failed: %v", err)
		}
	}
}
