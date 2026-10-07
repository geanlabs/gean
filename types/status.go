package types

import (
	"encoding/binary"
	"fmt"
)

const statusSize = 80

// Status is the req/resp status message: a node's finalized checkpoint and
// head, which peers compare to decide whether to sync.
type Status struct {
	FinalizedRoot [32]byte
	FinalizedSlot uint64
	HeadRoot      [32]byte
	HeadSlot      uint64
}

func (s *Status) MarshalSSZ() []byte {
	buf := make([]byte, statusSize)
	copy(buf[0:32], s.FinalizedRoot[:])
	binary.LittleEndian.PutUint64(buf[32:40], s.FinalizedSlot)
	copy(buf[40:72], s.HeadRoot[:])
	binary.LittleEndian.PutUint64(buf[72:80], s.HeadSlot)
	return buf
}

func (s *Status) UnmarshalSSZ(buf []byte) error {
	if len(buf) != statusSize {
		return fmt.Errorf("status message has %d bytes, want %d", len(buf), statusSize)
	}
	copy(s.FinalizedRoot[:], buf[0:32])
	s.FinalizedSlot = binary.LittleEndian.Uint64(buf[32:40])
	copy(s.HeadRoot[:], buf[40:72])
	s.HeadSlot = binary.LittleEndian.Uint64(buf[72:80])
	return nil
}
