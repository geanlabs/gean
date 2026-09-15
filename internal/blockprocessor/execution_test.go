package blockprocessor

import (
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/blockbuilder"
	"github.com/geanlabs/gean/internal/genesis"
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/types"
)

// The import path composes the transition phase by phase for metrics. A block
// built by the proposer, which runs the full transition, must import here with
// the state root it committed to; a phase missing from the composition shows
// up as a root mismatch on every block that carries a payload.
func TestTransitionStateImportsBuiltBlockWithPayload(t *testing.T) {
	cfg := &genesis.GenesisConfig{
		GenesisTime:          1_700_000_000,
		ExecutionGenesisHash: "0x" + strings.Repeat("3e", 32),
		GenesisValidators: []genesis.GenesisValidatorEntry{
			{AttestationPubkey: strings.Repeat("aa", 32), ProposalPubkey: strings.Repeat("bb", 32)},
		},
	}
	head, err := cfg.GenesisState()
	if err != nil {
		t.Fatal(err)
	}
	elGenesis, _ := cfg.ExecutionGenesisBlockHash()
	headHeader := *head.LatestBlockHeader
	headHeader.StateRoot, _ = head.HashTreeRoot()
	parentRoot, _ := headHeader.HashTreeRoot()

	payload := &types.ExecutionPayload{
		ParentHash:    elGenesis,
		Timestamp:     statetransition.ComputeTimeAtSlot(cfg.GenesisTime, 1),
		BlockHash:     [32]byte{0xE1},
		BlockNumber:   1,
		GasLimit:      30_000_000,
		BaseFeePerGas: [32]byte{0x07},
		Transactions:  [][]byte{{0x02, 0x01}},
		ExtraData:     []byte("geth"),
	}

	result, err := blockbuilder.Build(blockbuilder.Input{
		HeadState:        head,
		Slot:             1,
		ProposerIndex:    types.ProposerIndex(1, 1),
		ParentRoot:       parentRoot,
		KnownBlockRoots:  blockbuilder.RootSet{},
		ExecutionPayload: payload,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	post, err := transitionState(head, result.Block)
	if err != nil {
		t.Fatalf("import of a built block failed: %v", err)
	}
	if post.LatestExecutionPayloadHeader.BlockHash != payload.BlockHash {
		t.Fatal("imported state did not cache the payload header")
	}

	// And a payload that does not chain is refused on import, not just by
	// the plain transition.
	bad := *result.Block
	badBody := *bad.Body
	badBody.ExecutionPayload.ParentHash = [32]byte{0xFF}
	bad.Body = &badBody
	if _, err := transitionState(head, &bad); err == nil {
		t.Fatal("import must run the execution payload checks")
	}
}
