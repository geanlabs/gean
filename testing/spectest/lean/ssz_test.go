//go:build spectests

package lean

import (
	"bytes"
	"encoding/json"
	"testing"

	ssz "github.com/ferranbt/fastssz"

	"github.com/geanlabs/gean/internal/lean/p2p"
	"github.com/geanlabs/gean/internal/lean/specfixtures"
	"github.com/geanlabs/gean/internal/lean/types"
)

type sszFixture struct {
	specfixtures.Header
	TypeName string `json:"typeName"`
	// Value is the decoded form, for readers; serialized and root are what the
	// codec is checked against.
	Value           json.RawMessage `json:"value"`
	Serialized      string          `json:"serialized"`
	Root            string          `json:"root"`
	RejectionReason string          `json:"rejectionReason"`
	RawBytes        string          `json:"rawBytes"`
}

// sszCodec is what sszgen generates for every gean SSZ type.
type sszCodec interface {
	UnmarshalSSZ(buf []byte) error
	MarshalSSZ() ([]byte, error)
	HashTreeRoot() ([32]byte, error)
}

// sszTypes maps a spec type name to a new gean value of that type.
var sszTypes = map[string]func() sszCodec{
	"AggregatedAttestation":       func() sszCodec { return new(types.AggregatedAttestation) },
	"Attestation":                 func() sszCodec { return new(types.Attestation) },
	"AttestationData":             func() sszCodec { return new(types.AttestationData) },
	"Block":                       func() sszCodec { return new(types.Block) },
	"BlockBody":                   func() sszCodec { return new(types.BlockBody) },
	"BlockHeader":                 func() sszCodec { return new(types.BlockHeader) },
	"BlocksByRootRequest":         func() sszCodec { return new(types.BlocksByRootRequest) },
	"Checkpoint":                  func() sszCodec { return new(types.Checkpoint) },
	"Config":                      func() sszCodec { return new(types.ChainConfig) },
	"MultiMessageAggregate":       func() sszCodec { return new(types.MultiMessageAggregate) },
	"SignedAggregatedAttestation": func() sszCodec { return new(types.SignedAggregatedAttestation) },
	"SignedAttestation":           func() sszCodec { return new(types.SignedAttestation) },
	"SignedBlock":                 func() sszCodec { return new(types.SignedBlock) },
	"SingleMessageAggregate":      func() sszCodec { return new(types.SingleMessageAggregate) },
	"State":                       func() sszCodec { return new(types.State) },
	"Status":                      func() sszCodec { return new(sszStatusAdapter) },
	"Validator":                   func() sszCodec { return new(types.Validator) },
}

// sszRejectionDecoders decode the malformed input of a rejection case. gean's
// SSZ is generated code, so only rules it calls into the SSZ library for
// have a decoder to check.
var sszRejectionDecoders = map[string]func([]byte) error{
	"DecodeBitlist8": func(buf []byte) error { return ssz.ValidateBitlist(buf, 8) },
}

// runSSZ decodes the spec's encoding into gean's type, re-encodes it, and
// checks both the bytes and the hash tree root.
func runSSZ(t *testing.T, fixture *sszFixture) {
	if poseidonStructureTypes[fixture.TypeName] {
		skip(t, skipPoseidonXMSS, fixture.TypeName+" is a Poseidon XMSS structure")
	}
	if fixture.RejectionReason != "" {
		runSSZRejection(t, fixture)
		return
	}
	if fixture.TypeName == "AttestationSubnets" {
		skip(t, skipFollowsEthlambda, "gean sizes attnets by the committee count, not as a 64-bit Bitvector")
	}
	newValue, ok := sszTypes[fixture.TypeName]
	if !ok {
		skip(t, skipNotImplemented, "SSZ type "+fixture.TypeName)
	}

	encoding := mustParse(t, specfixtures.ParseHexBytes, fixture.Serialized)
	value := newValue()
	if err := value.UnmarshalSSZ(encoding); err != nil {
		t.Fatalf("decode: %v", err)
	}
	reencoded, err := value.MarshalSSZ()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.Equal(reencoded, encoding) {
		t.Errorf("re-encoding: got %x, want %x", reencoded, encoding)
	}
	root, err := value.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash tree root: %v", err)
	}
	if want := mustParse(t, specfixtures.ParseHexRoot, fixture.Root); root != want {
		t.Errorf("hash tree root: got 0x%x, want 0x%x", root, want)
	}
}

// runSSZRejection checks that a malformed encoding fails to decode.
func runSSZRejection(t *testing.T, fixture *sszFixture) {
	decode, ok := sszRejectionDecoders[fixture.TypeName]
	if !ok {
		skip(t, skipNotImplemented, "standalone decoder for "+fixture.TypeName)
	}
	if err := decode(mustParse(t, specfixtures.ParseHexBytes, fixture.RawBytes)); err == nil {
		t.Errorf("decoded malformed %s; want %s", fixture.TypeName, fixture.RejectionReason)
	}
}

// sszStatusAdapter runs p2p.StatusMessage, whose SSZ is hand-written and has
// no hash tree root, through sszCodec. The root is composed here from Status's
// layout, a container of two checkpoints.
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
