package store

import (
	"encoding/binary"
	"fmt"

	"github.com/geanlabs/gean/internal/storage"
)

// SigningRole names the XMSS key a signature is made with. Each validator has
// one key per role, so each role keeps its own signed-slot record.
type SigningRole byte

const (
	RoleAttestation SigningRole = iota
	RoleProposal
)

// ReserveSigningSlot records that validatorIDs are about to sign at slot with
// their role key, or refuses if any of them has already reserved slot or a
// later one, in which case nothing is recorded. The slot is the XMSS one-time
// key index, so signing it twice reuses the index. The record reaches stable
// storage before this returns, so a crash after signing cannot let the
// restarted node sign the same slot again.
func (s *ConsensusStore) ReserveSigningSlot(role SigningRole, slot uint64, validatorIDs []uint64) error {
	s.signedSlotsMu.Lock()
	defer s.signedSlotsMu.Unlock()

	rv, err := s.beginRead("reserve signing slot")
	if err != nil {
		return err
	}
	entries := make([]storage.KV, 0, len(validatorIDs))
	value := binary.BigEndian.AppendUint64(nil, slot)
	for _, vid := range validatorIDs {
		key := []byte{byte(role)}
		key = binary.BigEndian.AppendUint64(key, vid)
		signed, err := rv.Get(storage.TableSignedSlots, key)
		if err != nil {
			return fmt.Errorf("reserve signing slot: get: %w", err)
		}
		if signed != nil {
			if len(signed) != 8 {
				return fmt.Errorf("reserve signing slot: validator %d record has %d bytes", vid, len(signed))
			}
			if last := binary.BigEndian.Uint64(signed); last >= slot {
				return fmt.Errorf("reserve signing slot: validator %d has slot %d", vid, last)
			}
		}
		entries = append(entries, storage.KV{Key: key, Value: value})
	}

	wb, err := s.beginWrite("reserve signing slot")
	if err != nil {
		return err
	}
	if err := wb.PutBatch(storage.TableSignedSlots, entries); err != nil {
		return fmt.Errorf("reserve signing slot: put: %w", err)
	}
	if err := wb.CommitSync(); err != nil {
		return fmt.Errorf("reserve signing slot: commit: %w", err)
	}
	return nil
}
