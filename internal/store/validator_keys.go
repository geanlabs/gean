package store

import (
	"github.com/geanlabs/gean/internal/types"
)

// validatorKeysCacheSize bounds the cache. An attestation's target is the safe
// target checkpoint, which advances at most once per slot and often stands for
// several: a devnet node produced 53 attestations across 6 distinct target
// roots. Sixty-four leaves room for reorgs and for the spread of targets the
// rest of the network votes for without the cache ever becoming a memory item
// of its own — each entry is one 32-byte key per validator.
const validatorKeysCacheSize = 64

// ValidatorKeys is an immutable snapshot of one state's attestation public
// keys, in validator-index order.
//
// Accessors hand back arrays by value rather than exposing the slice, so a
// cached snapshot cannot be aliased and mutated by a caller. That is the whole
// reason this type exists instead of a cache in front of GetState: GetState
// returns a fresh state on every call because callers such as the block
// processor pass the result straight into StateTransition, which mutates it.
// Caching what GetState returns would let one of those callers corrupt the
// state that signature verification reads.
type ValidatorKeys struct {
	keys [][types.PubkeySize]byte
}

// Len reports the size of the validator registry.
func (v *ValidatorKeys) Len() int {
	if v == nil {
		return 0
	}
	return len(v.keys)
}

// AttestationPubkey returns the attestation public key of the validator at
// index, and whether that index is in the registry.
func (v *ValidatorKeys) AttestationPubkey(index uint64) ([types.PubkeySize]byte, bool) {
	if v == nil || index >= uint64(len(v.keys)) {
		return [types.PubkeySize]byte{}, false
	}
	return v.keys[index], true
}

// ValidatorKeys returns the attestation public keys of the validator registry
// at root, or nil when no state is stored for that root.
//
// Signature verification needs one 32-byte key, but reaching it through
// GetState decodes the entire state to get there. A state is dominated by
// HistoricalBlockHashes, which holds one 32-byte entry per slot since genesis:
// about 628 KB at slot 20,000, decoded in 2.1 ms across 20,023 allocations,
// and growing for as long as the chain does. Verification never reads any of
// it. Gossip verification runs a few times a second on every node, so that was
// megabytes a second of garbage produced to reach a few hundred bytes.
//
// Caching by state root is sound because a root determines its state: an entry
// can go stale only by outliving the state it describes, which costs nothing
// beyond the bounded space it sits in.
func (s *ConsensusStore) ValidatorKeys(root [32]byte) *ValidatorKeys {
	if s == nil {
		return nil
	}

	s.validatorKeysMu.Lock()
	cached, ok := s.validatorKeys[root]
	s.validatorKeysMu.Unlock()
	if ok {
		return cached
	}

	// Built outside the lock: this is the expensive path, and holding the mutex
	// across it would serialise every verifier behind one decode.
	state := s.GetState(root)
	if state == nil {
		return nil
	}
	vk := &ValidatorKeys{keys: make([][types.PubkeySize]byte, len(state.Validators))}
	for i, validator := range state.Validators {
		if validator == nil {
			continue
		}
		vk.keys[i] = validator.AttestationPubkey
	}

	s.validatorKeysMu.Lock()
	defer s.validatorKeysMu.Unlock()
	// Two verifiers can miss on the same root concurrently. Keep whichever
	// landed first so every caller observes one snapshot per root.
	if existing, ok := s.validatorKeys[root]; ok {
		return existing
	}
	if s.validatorKeys == nil {
		s.validatorKeys = make(map[[32]byte]*ValidatorKeys, validatorKeysCacheSize)
	}
	if len(s.validatorKeysOrder) >= validatorKeysCacheSize {
		delete(s.validatorKeys, s.validatorKeysOrder[0])
		s.validatorKeysOrder = s.validatorKeysOrder[1:]
	}
	s.validatorKeys[root] = vk
	s.validatorKeysOrder = append(s.validatorKeysOrder, root)
	return vk
}
