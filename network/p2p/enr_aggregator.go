package p2p

import (
	"io"

	"github.com/ethereum/go-ethereum/rlp"
)

// AggregatorEntry advertises whether the node is an aggregator.
// ENR key: "is_aggregator" with value 0x01 (true) or 0x00 (false).
type AggregatorEntry bool

func (e AggregatorEntry) ENRKey() string { return "is_aggregator" }

func (e AggregatorEntry) EncodeRLP(w io.Writer) error {
	var v byte
	if e {
		v = 0x01
	}
	return rlp.Encode(w, v)
}

func (e *AggregatorEntry) DecodeRLP(s *rlp.Stream) error {
	var v byte
	if err := s.Decode(&v); err != nil {
		return err
	}
	*e = AggregatorEntry(v == 0x01)
	return nil
}
