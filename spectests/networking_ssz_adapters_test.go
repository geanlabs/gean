//go:build spectests

package spectests

import (
	ssz "github.com/ferranbt/fastssz"

	"github.com/geanlabs/gean/p2p"
	"github.com/geanlabs/gean/types"
)

// sszStatusAdapter wraps p2p.StatusMessage so the SSZ spec-test harness can
// drive it through the same sszCodec interface as consensus containers.
//
// p2p.StatusMessage is hand-rolled SSZ in the networking layer (no
// HashTreeRoot — req/resp messages aren't merkleized on the consensus hot
// path). For the leanSpec networking SSZ fixtures we compose the hash here:
// Status is a Container { finalized: Checkpoint, head: Checkpoint }, and
// types.Checkpoint already exposes HashTreeRootWith via sszgen.
type sszStatusAdapter struct {
	Finalized types.Checkpoint
	Head      types.Checkpoint
}

func (s *sszStatusAdapter) MarshalSSZ() ([]byte, error) {
	msg := p2p.StatusMessage{
		FinalizedRoot: s.Finalized.Root,
		FinalizedSlot: s.Finalized.Slot,
		HeadRoot:      s.Head.Root,
		HeadSlot:      s.Head.Slot,
	}
	return msg.MarshalSSZ(), nil
}

func (s *sszStatusAdapter) UnmarshalSSZ(buf []byte) error {
	var msg p2p.StatusMessage
	if err := msg.UnmarshalSSZ(buf); err != nil {
		return err
	}
	s.Finalized = types.Checkpoint{Root: msg.FinalizedRoot, Slot: msg.FinalizedSlot}
	s.Head = types.Checkpoint{Root: msg.HeadRoot, Slot: msg.HeadSlot}
	return nil
}

func (s *sszStatusAdapter) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(s)
}

func (s *sszStatusAdapter) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(s)
}

func (s *sszStatusAdapter) HashTreeRootWith(hh ssz.HashWalker) error {
	indx := hh.Index()
	if err := s.Finalized.HashTreeRootWith(hh); err != nil {
		return err
	}
	if err := s.Head.HashTreeRootWith(hh); err != nil {
		return err
	}
	hh.Merkleize(indx)
	return nil
}

// sszBlocksByRootRequestAdapter wraps the standalone encode/decode functions
// from p2p (gean models BlocksByRootRequest as a [][32]byte rather than a
// struct) so they can flow through the sszCodec contract used by the SSZ
// spec-test harness. Hash composition: Container { roots: List[Bytes32, MAX_REQUEST_BLOCKS] }.
type sszBlocksByRootRequestAdapter struct {
	Roots [][32]byte
}

func (b *sszBlocksByRootRequestAdapter) MarshalSSZ() ([]byte, error) {
	return p2p.EncodeBlocksByRootRequest(b.Roots), nil
}

func (b *sszBlocksByRootRequestAdapter) UnmarshalSSZ(buf []byte) error {
	roots, err := p2p.DecodeBlocksByRootRequest(buf)
	if err != nil {
		return err
	}
	b.Roots = roots
	return nil
}

func (b *sszBlocksByRootRequestAdapter) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(b)
}

func (b *sszBlocksByRootRequestAdapter) GetTree() (*ssz.Node, error) {
	return ssz.ProofTree(b)
}

func (b *sszBlocksByRootRequestAdapter) HashTreeRootWith(hh ssz.HashWalker) error {
	indx := hh.Index()
	{
		subIndx := hh.Index()
		for i := range b.Roots {
			hh.Append(b.Roots[i][:])
		}
		hh.MerkleizeWithMixin(subIndx, uint64(len(b.Roots)), uint64(types.MaxRequestBlocks))
	}
	hh.Merkleize(indx)
	return nil
}
