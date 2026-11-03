package store

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/geanlabs/gean/internal/storage"
)

// SigningRole names which of a validator's two XMSS keys is signing.
type SigningRole byte

const (
	SigningAttestation SigningRole = iota
	SigningProposal
)

const signingRecordSize = 8 + 32

// ErrSigningConflict means a signature would reuse or go behind an XMSS slot
// this data directory has already signed for.
var ErrSigningConflict = errors.New("signing conflict")

// ReserveSignature must succeed before a key signs. XMSS must never sign two
// messages at one slot, so it durably records the validator's highest signed
// slot and message, and refuses an earlier slot or a different message at that
// slot. The identical message is allowed: signing is derandomized, so repeating
// it is harmless.
//
// One record per validator and key keeps the table small, and needs no pruning.
// The record lives in the data directory, so wiping it also removes this
// protection. Each key signs from one goroutine, so read then write cannot race.
func (s *ConsensusStore) ReserveSignature(role SigningRole, validatorID, slot uint64, message [32]byte) error {
	key := make([]byte, 9)
	key[0] = byte(role)
	binary.BigEndian.PutUint64(key[1:], validatorID)

	rv, err := s.beginRead("reserve signature")
	if err != nil {
		return err
	}
	record, err := rv.Get(storage.TableSigningRecords, key)
	if err != nil {
		return fmt.Errorf("reserve signature: read: %w", err)
	}
	if record != nil {
		if len(record) != signingRecordSize {
			return fmt.Errorf("reserve signature: record is %d bytes, want %d", len(record), signingRecordSize)
		}
		signedSlot := binary.BigEndian.Uint64(record)
		if slot == signedSlot && [32]byte(record[8:]) == message {
			return nil
		}
		if slot <= signedSlot {
			return fmt.Errorf("%w: validator %d role %d slot %d, already signed slot %d", ErrSigningConflict, validatorID, role, slot, signedSlot)
		}
	}

	value := make([]byte, signingRecordSize)
	binary.BigEndian.PutUint64(value, slot)
	copy(value[8:], message[:])
	wb, err := s.beginWrite("reserve signature")
	if err != nil {
		return err
	}
	if err := wb.PutBatch(storage.TableSigningRecords, []storage.KV{{Key: key, Value: value}}); err != nil {
		return fmt.Errorf("reserve signature: put: %w", err)
	}
	if err := wb.CommitSync(); err != nil {
		return fmt.Errorf("reserve signature: commit: %w", err)
	}
	return nil
}
