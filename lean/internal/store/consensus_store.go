package store

import (
	"sync"
	"sync/atomic"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/lean/crypto/xmss"
)

type ConsensusStore struct {
	Backend     storage.Backend
	PubKeyCache *xmss.PubKeyCache

	// maxBlockSlot is the highest slot of any block header this store has
	// written: a high-water mark, not a live maximum over the table. It answers
	// MaxStoredBlockSlot, which the duty gate reads up to three times a slot and
	// which used to scan and SSZ-decode every header in TableBlockHeaders.
	//
	// It deliberately covers the same set the scan did — imported blocks and
	// blocks stored while pending — because the duty gate's network-stall
	// carve-out depends on it. Narrowing it to imported blocks only would let a
	// node that restarted holding a pending block at a near-current slot read
	// its own import lag as a network stall and resume duties on a stale head.
	//
	// The one behavioural difference from the scan: pruning the highest-slot
	// header no longer lowers the answer. That errs toward reporting the network
	// as alive, which is the direction that keeps the gate closed rather than
	// opening it onto a dead fork.
	maxBlockSlot atomic.Uint64
	// maxBlockSlotSeeded latches only once a seeding scan has completed without
	// error. A sync.Once would latch on failure too, and a scan that returns 0
	// because the read view could not be opened — or that stopped part-way
	// through the table — would permanently understate the mark. That is not
	// merely a bad gauge: an understated mark inflates the duty gate's computed
	// network lag, which can trip the network-stall carve-out and let the node
	// resume duties on a stale head.
	maxBlockSlotSeeded atomic.Bool
	maxBlockSlotSeedMu sync.Mutex

	// statesPrunedBelow is the slot below which PruneStaleStates has already
	// deleted the head's stale ancestor states, and statesKeptJustifiedSlot the
	// justified slot it last kept. Block import lowers the mark; both run on the
	// dispatch loop, the only reader and writer.
	statesPrunedBelow       uint64
	statesKeptJustifiedSlot uint64

	// validatorKeys caches each state's attestation public keys by state root so
	// signature verification does not decode a whole state to read one key. See
	// ValidatorKeys. validatorKeysOrder records insertion order and is the
	// eviction queue; both are guarded by validatorKeysMu.
	validatorKeysMu    sync.Mutex
	validatorKeys      map[[32]byte]*ValidatorKeys
	validatorKeysOrder [][32]byte

	// stateSummaries holds the StateSummary of every stored state, keyed by block
	// root, so the per-slot paths read memory rather than decoding states.
	// Entries are added when a state is written and dropped when it is pruned.
	stateSummariesMu sync.Mutex
	stateSummaries   map[[32]byte]StateSummary

	// blocks keeps block metadata and the canonical chain in memory for chain
	// walks. See blockIndex.
	blocks blockIndex
}

// ObserveStoredBlockSlot raises the stored-block high-water mark. Safe from any
// goroutine: block import runs on the dispatch loop, but pending-block writes
// and the test driver do not.
func (s *ConsensusStore) ObserveStoredBlockSlot(slot uint64) {
	if s == nil {
		return
	}
	for {
		current := s.maxBlockSlot.Load()
		if slot <= current {
			return
		}
		if s.maxBlockSlot.CompareAndSwap(current, slot) {
			return
		}
	}
}

func NewConsensusStore(backend storage.Backend) *ConsensusStore {
	return &ConsensusStore{
		Backend:        backend,
		PubKeyCache:    xmss.NewPubKeyCache(),
		validatorKeys:  make(map[[32]byte]*ValidatorKeys, validatorKeysCacheSize),
		stateSummaries: make(map[[32]byte]StateSummary),
	}
}
