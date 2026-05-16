package forkchoice

import (
	"testing"

	"github.com/geanlabs/gean/types"
)

func root(b byte) [32]byte {
	var r [32]byte
	r[0] = b
	return r
}

func makeAttData(headRoot [32]byte, slot uint64) *types.AttestationData {
	return &types.AttestationData{
		Slot:   slot,
		Head:   &types.Checkpoint{Root: headRoot, Slot: slot},
		Target: &types.Checkpoint{},
		Source: &types.Checkpoint{},
	}
}

// --- Spec implementation tests (rs tests) ---

func TestSpecComputeBlockWeights(t *testing.T) {
	// Chain: root_a (slot 0) -> root_b (slot 1) -> root_c (slot 2)
	rootA, rootB, rootC := root(1), root(2), root(3)
	blocks := map[[32]byte]BlockInfo{
		rootA: {Slot: 0, ParentRoot: [32]byte{}},
		rootB: {Slot: 1, ParentRoot: rootA},
		rootC: {Slot: 2, ParentRoot: rootB},
	}
	attestations := map[uint64]*types.AttestationData{
		0: makeAttData(rootC, 2),
		1: makeAttData(rootB, 1),
	}

	weights := SpecComputeBlockWeights(0, blocks, attestations)

	// rootC: 1 vote (validator 0)
	if weights[rootC] != 1 {
		t.Fatalf("rootC weight: expected 1, got %d", weights[rootC])
	}
	// rootB: 2 votes (validator 0 walks through + validator 1 direct)
	if weights[rootB] != 2 {
		t.Fatalf("rootB weight: expected 2, got %d", weights[rootB])
	}
	// rootA: at slot 0 = start_slot, not counted
	if weights[rootA] != 0 {
		t.Fatalf("rootA weight: expected 0, got %d", weights[rootA])
	}
}

func TestSpecComputeBlockWeightsEmpty(t *testing.T) {
	weights := SpecComputeBlockWeights(0, nil, nil)
	if len(weights) != 0 {
		t.Fatal("expected empty weights")
	}
}

func TestSpecLMDGhostLinearChain(t *testing.T) {
	rootA, rootB, rootC := root(1), root(2), root(3)
	blocks := map[[32]byte]BlockInfo{
		rootA: {Slot: 0, ParentRoot: [32]byte{}},
		rootB: {Slot: 1, ParentRoot: rootA},
		rootC: {Slot: 2, ParentRoot: rootB},
	}
	attestations := map[uint64]*types.AttestationData{
		0: makeAttData(rootC, 2),
	}

	head, _ := SpecComputeLMDGhostHead(rootA, blocks, attestations, 0)
	if head != rootC {
		t.Fatalf("expected rootC, got %x", head[:4])
	}
}

func TestSpecLMDGhostForkHeavier(t *testing.T) {
	rootA, rootB, rootC := root(1), root(2), root(3)
	blocks := map[[32]byte]BlockInfo{
		rootA: {Slot: 0, ParentRoot: [32]byte{}},
		rootB: {Slot: 1, ParentRoot: rootA},
		rootC: {Slot: 1, ParentRoot: rootA},
	}
	// 2 votes for rootB, 1 for rootC -> rootB wins
	attestations := map[uint64]*types.AttestationData{
		0: makeAttData(rootB, 1),
		1: makeAttData(rootB, 1),
		2: makeAttData(rootC, 1),
	}

	head, _ := SpecComputeLMDGhostHead(rootA, blocks, attestations, 0)
	if head != rootB {
		t.Fatalf("expected rootB (heavier), got %x", head[:4])
	}
}

func TestSpecLMDGhostTiebreakLexicographic(t *testing.T) {
	rootA := root(1)
	rootB := root(2) // smaller
	rootC := root(3) // larger -> wins tiebreak
	blocks := map[[32]byte]BlockInfo{
		rootA: {Slot: 0, ParentRoot: [32]byte{}},
		rootB: {Slot: 1, ParentRoot: rootA},
		rootC: {Slot: 1, ParentRoot: rootA},
	}
	// Equal weight: 1 vote each -> lexicographic tiebreak, rootC > rootB
	attestations := map[uint64]*types.AttestationData{
		0: makeAttData(rootB, 1),
		1: makeAttData(rootC, 1),
	}

	head, _ := SpecComputeLMDGhostHead(rootA, blocks, attestations, 0)
	if head != rootC {
		t.Fatalf("expected rootC (lexicographic tiebreak), got %x", head[:4])
	}
}

// --- Proto-array tests ---

func TestProtoArrayLinearChain(t *testing.T) {
	rootA, rootB, rootC := root(1), root(2), root(3)
	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)
	fc.OnBlock(2, rootC, rootB)

	// Validator 0 attests to rootC
	fc.Votes.SetKnown(0, fc.NodeIndex(rootC), 2, makeAttData(rootC, 2))

	head := fc.UpdateHead(rootA)
	if head != rootC {
		t.Fatalf("expected rootC, got %x", head[:4])
	}
}

func TestProtoArrayForkHeavier(t *testing.T) {
	rootA, rootB, rootC := root(1), root(2), root(3)
	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)
	fc.OnBlock(1, rootC, rootA)

	// 2 votes for rootB, 1 for rootC
	fc.Votes.SetKnown(0, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	fc.Votes.SetKnown(1, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	fc.Votes.SetKnown(2, fc.NodeIndex(rootC), 1, makeAttData(rootC, 1))

	head := fc.UpdateHead(rootA)
	if head != rootB {
		t.Fatalf("expected rootB (heavier), got %x", head[:4])
	}
}

func TestProtoArrayTiebreakLexicographic(t *testing.T) {
	rootA := root(1)
	rootB := root(2)
	rootC := root(3) // larger -> wins
	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)
	fc.OnBlock(1, rootC, rootA)

	fc.Votes.SetKnown(0, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	fc.Votes.SetKnown(1, fc.NodeIndex(rootC), 1, makeAttData(rootC, 1))

	head := fc.UpdateHead(rootA)
	if head != rootC {
		t.Fatalf("expected rootC (tiebreak), got %x", head[:4])
	}
}

func TestProtoArrayNoAttestations(t *testing.T) {
	rootA := root(1)
	fc := New(0, rootA, [32]byte{})
	head := fc.UpdateHead(rootA)
	if head != rootA {
		t.Fatalf("expected rootA with no attestations, got %x", head[:4])
	}
}

func TestProtoArrayVoteChange(t *testing.T) {
	rootA, rootB, rootC := root(1), root(2), root(3)
	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)
	fc.OnBlock(1, rootC, rootA)

	// Initially vote for rootB
	fc.Votes.SetKnown(0, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	head := fc.UpdateHead(rootA)
	if head != rootB {
		t.Fatalf("expected rootB initially, got %x", head[:4])
	}

	// Change vote to rootC
	fc.Votes.SetKnown(0, fc.NodeIndex(rootC), 1, makeAttData(rootC, 1))
	head = fc.UpdateHead(rootA)
	if head != rootC {
		t.Fatalf("expected rootC after vote change, got %x", head[:4])
	}
}

func TestProtoArrayPrune(t *testing.T) {
	rootA, rootB, rootC := root(1), root(2), root(3)
	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)
	fc.OnBlock(2, rootC, rootB)

	if fc.Array.Len() != 3 {
		t.Fatalf("expected 3 nodes, got %d", fc.Array.Len())
	}

	fc.Prune(rootB)

	if fc.Array.Len() != 2 {
		t.Fatalf("expected 2 nodes after prune, got %d", fc.Array.Len())
	}
	if fc.NodeIndex(rootA) != -1 {
		t.Fatal("rootA should be pruned")
	}
	if fc.NodeIndex(rootB) < 0 {
		t.Fatal("rootB should still exist")
	}
}

func TestProtoArrayDeepChain(t *testing.T) {
	roots := make([][32]byte, 10)
	for i := range roots {
		roots[i] = root(byte(i + 1))
	}
	fc := New(0, roots[0], [32]byte{})
	for i := 1; i < 10; i++ {
		fc.OnBlock(uint64(i), roots[i], roots[i-1])
	}

	// Attest to tip
	fc.Votes.SetKnown(0, fc.NodeIndex(roots[9]), 9, makeAttData(roots[9], 9))
	head := fc.UpdateHead(roots[0])
	if head != roots[9] {
		t.Fatalf("expected root[9], got %x", head[:4])
	}
}

// --- Debug oracle: verify proto-array matches spec ---

func TestDebugOracleLinearChain(t *testing.T) {
	rootA, rootB, rootC := root(1), root(2), root(3)

	// Spec
	blocks := map[[32]byte]BlockInfo{
		rootA: {Slot: 0, ParentRoot: [32]byte{}},
		rootB: {Slot: 1, ParentRoot: rootA},
		rootC: {Slot: 2, ParentRoot: rootB},
	}
	attestations := map[uint64]*types.AttestationData{
		0: makeAttData(rootC, 2),
		1: makeAttData(rootB, 1),
	}
	specHead, _ := SpecComputeLMDGhostHead(rootA, blocks, attestations, 0)

	// Proto-array
	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)
	fc.OnBlock(2, rootC, rootB)
	fc.Votes.SetKnown(0, fc.NodeIndex(rootC), 2, makeAttData(rootC, 2))
	fc.Votes.SetKnown(1, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	protoHead := fc.UpdateHead(rootA)

	if specHead != protoHead {
		t.Fatalf("ORACLE MISMATCH: spec=%x proto=%x", specHead[:4], protoHead[:4])
	}
}

func TestDebugOracleFork(t *testing.T) {
	rootA, rootB, rootC := root(1), root(2), root(3)

	blocks := map[[32]byte]BlockInfo{
		rootA: {Slot: 0, ParentRoot: [32]byte{}},
		rootB: {Slot: 1, ParentRoot: rootA},
		rootC: {Slot: 1, ParentRoot: rootA},
	}
	attestations := map[uint64]*types.AttestationData{
		0: makeAttData(rootB, 1),
		1: makeAttData(rootB, 1),
		2: makeAttData(rootC, 1),
	}
	specHead, _ := SpecComputeLMDGhostHead(rootA, blocks, attestations, 0)

	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)
	fc.OnBlock(1, rootC, rootA)
	fc.Votes.SetKnown(0, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	fc.Votes.SetKnown(1, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	fc.Votes.SetKnown(2, fc.NodeIndex(rootC), 1, makeAttData(rootC, 1))
	protoHead := fc.UpdateHead(rootA)

	if specHead != protoHead {
		t.Fatalf("ORACLE MISMATCH: spec=%x proto=%x", specHead[:4], protoHead[:4])
	}
}

func TestDebugOracleTiebreak(t *testing.T) {
	rootA := root(1)
	rootB := root(2)
	rootC := root(3)

	blocks := map[[32]byte]BlockInfo{
		rootA: {Slot: 0, ParentRoot: [32]byte{}},
		rootB: {Slot: 1, ParentRoot: rootA},
		rootC: {Slot: 1, ParentRoot: rootA},
	}
	attestations := map[uint64]*types.AttestationData{
		0: makeAttData(rootB, 1),
		1: makeAttData(rootC, 1),
	}
	specHead, _ := SpecComputeLMDGhostHead(rootA, blocks, attestations, 0)

	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)
	fc.OnBlock(1, rootC, rootA)
	fc.Votes.SetKnown(0, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	fc.Votes.SetKnown(1, fc.NodeIndex(rootC), 1, makeAttData(rootC, 1))
	protoHead := fc.UpdateHead(rootA)

	if specHead != protoHead {
		t.Fatalf("ORACLE MISMATCH: spec=%x proto=%x", specHead[:4], protoHead[:4])
	}
}

func TestReorgDepth(t *testing.T) {
	// Anchor (slot 0) -> A (slot 1) -> A2 (slot 2) -> A3 (slot 3)
	//                  \-> B (slot 1) -> B2 (slot 2)
	anchor, a, a2, a3, b, b2 := root(1), root(2), root(3), root(4), root(5), root(6)
	fc := New(0, anchor, [32]byte{})
	fc.OnBlock(1, a, anchor)
	fc.OnBlock(2, a2, a)
	fc.OnBlock(3, a3, a2)
	fc.OnBlock(1, b, anchor)
	fc.OnBlock(2, b2, b)

	cases := []struct {
		name      string
		oldHead   [32]byte
		newHead   [32]byte
		wantDepth uint64
	}{
		{"same head no reorg", a3, a3, 0},
		{"normal extension a → a2 (no reorg)", a, a2, 0},
		{"1-block reorg a → b at slot 1", a, b, 1},
		{"2-block reorg a2 → b2 at slot 2", a2, b2, 2},
		{"3-block reorg a3 → b at slot 1", a3, b, 3},
		{"unknown old root", root(99), a, 0},
		{"unknown new root", a, root(99), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fc.ReorgDepth(tc.oldHead, tc.newHead)
			if got != tc.wantDepth {
				t.Fatalf("ReorgDepth(old=%x, new=%x) = %d, want %d",
					tc.oldHead[:1], tc.newHead[:1], got, tc.wantDepth)
			}
		})
	}
}

func TestAnchorParentRootPreserved(t *testing.T) {
	anchorRoot := root(7)
	anchorParent := root(6)

	fc := New(12, anchorRoot, anchorParent)

	nodes := fc.Array.Nodes()
	if len(nodes) != 1 {
		t.Fatalf("expected 1 anchor node, got %d", len(nodes))
	}
	if nodes[0].ParentRoot != anchorParent {
		t.Fatalf("anchor ParentRoot = %x, want %x", nodes[0].ParentRoot, anchorParent)
	}
	if nodes[0].Root != anchorRoot {
		t.Fatalf("anchor Root = %x, want %x", nodes[0].Root, anchorRoot)
	}
	if nodes[0].Slot != 12 {
		t.Fatalf("anchor Slot = %d, want 12", nodes[0].Slot)
	}
}

func TestGenesisAnchorParentRootZero(t *testing.T) {
	genesisRoot := root(1)

	fc := New(0, genesisRoot, [32]byte{})

	nodes := fc.Array.Nodes()
	if nodes[0].ParentRoot != ([32]byte{}) {
		t.Fatalf("genesis ParentRoot = %x, want zero", nodes[0].ParentRoot)
	}
}
